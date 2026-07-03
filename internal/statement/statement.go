// Package statement renders a monthly PDF statement for one account: a
// summary, the same charts shown on the dashboard, and the period's
// transaction list. It's the single source of truth for statement content,
// used identically by the on-demand download endpoint and the monthly job.
package statement

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/go-pdf/fpdf"
	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorCredit = drawing.ColorFromHex("1a7f37")
	colorDebit  = drawing.ColorFromHex("c62828")
	colorLine   = drawing.ColorFromHex("2f6fed")
	colorAxis   = drawing.ColorFromHex("6b7280")
)

// chartScale renders chart PNGs at N times the base pixel size (with a
// matching DPI bump so point-sized text and a matching stroke-width bump so
// lines stay proportional) before they're embedded at a fixed physical size
// in the PDF — the same "render at Nx, display at 1x" trick used for
// crisp/retina raster images, since the PDF page size doesn't change.
const (
	chartScale      = 3
	chartBaseWidth  = 700
	chartBaseHeight = 300
	chartWidth      = chartBaseWidth * chartScale
	chartHeight     = chartBaseHeight * chartScale
	chartDPI        = chart.DefaultDPI * chartScale
	chartStrokeW    = 2 * chartScale
)

// chartPadding leaves just enough room on the right for go-chart's axis (it
// renders the primary Y-axis — value or percentage ticks — on the right side
// by default) so its labels don't collide with the last X-axis tick, which is
// otherwise anchored at the same edge. Kept as tight as that allows so the
// plotted data fills as much of the chart box as possible.
var chartPadding = chart.Box{Top: 20 * chartScale, Left: 4 * chartScale, Right: 40 * chartScale, Bottom: 30 * chartScale}

// Deps is what Generate needs from the running server.
type Deps struct {
	Store *store.Store
	Cfg   config.Config
}

// PeriodBounds returns the [from, to) UTC month window containing `t`.
func PeriodBounds(t time.Time) (from, to time.Time) {
	from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// Generate builds a PDF statement for one account covering the calendar month
// containing `period`.
func Generate(ctx context.Context, d Deps, tenant domain.Tenant, acct domain.Account, holderName string, period time.Time) ([]byte, error) {
	from, to := PeriodBounds(period)

	balHist, err := d.Store.BalanceHistory(ctx, tenant.ID, acct.ID, from, to, "day")
	if err != nil {
		return nil, fmt.Errorf("balance history: %w", err)
	}
	earnRedeem, err := d.Store.EarnRedeemSummary(ctx, tenant.ID, acct.ID, from, to, "week")
	if err != nil {
		return nil, fmt.Errorf("earn/redeem summary: %w", err)
	}
	leaderboard, err := d.Store.TaskLeaderboard(ctx, tenant.ID, &acct.ID, from, to)
	if err != nil {
		return nil, fmt.Errorf("task leaderboard: %w", err)
	}
	txs, err := d.Store.ListTransactionsInRange(ctx, tenant.ID, acct.ID, from, to)
	if err != nil {
		return nil, fmt.Errorf("transactions: %w", err)
	}

	opening := int64(0)
	closing := int64(0)
	if len(balHist) > 0 {
		opening = balHist[0].BalanceMinor
		closing = balHist[len(balHist)-1].BalanceMinor
	}
	var earned, redeemed int64
	for _, b := range earnRedeem {
		earned += b.EarnedMinor
		redeemed += b.RedeemedMinor
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	brand := d.Cfg.Branding
	writeHeader(pdf, brand.ProductName, tenant.Name, holderName, acct.ID, period)
	writeSummary(pdf, brand.CoinCode, opening, earned, redeemed, closing)

	if len(balHist) > 1 {
		if img, err := balanceChartPNG(balHist); err == nil {
			writeChart(pdf, "Balance over time", img, nil)
		}
	}
	if hasActivity(earnRedeem) {
		if img, err := earnRedeemChartPNG(earnRedeem); err == nil {
			legend := [][2]string{{"Earned", "1a7f37"}, {"Redeemed", "c62828"}}
			writeChart(pdf, "Earned vs. redeemed", img, legend)
		}
	}
	if len(leaderboard) > 0 {
		writeLeaderboard(pdf, "Top chores this month", leaderboard)
	}

	writeTransactions(pdf, txs)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func hasActivity(buckets []domain.EarnRedeemBucket) bool {
	for _, b := range buckets {
		if b.EarnedMinor != 0 || b.RedeemedMinor != 0 {
			return true
		}
	}
	return false
}

func writeHeader(pdf *fpdf.Fpdf, product, household, holder, acctID string, period time.Time) {
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(0, 10, product, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(90, 90, 90)
	pdf.CellFormat(0, 7, "Monthly Statement - "+period.Format("January 2006"), "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Household: %s", household), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Account holder: %s", holder), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Account No. %s", shortID(acctID)), "", 1, "L", false, 0, "")
	pdf.Ln(4)
}

func writeSummary(pdf *fpdf.Fpdf, coinCode string, opening, earned, redeemed, closing int64) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Summary", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)

	rows := [][2]string{
		{"Opening balance", money.Format(opening) + " " + coinCode},
		{"Earned this period", "+" + money.Format(earned) + " " + coinCode},
		{"Redeemed this period", "-" + money.Format(redeemed) + " " + coinCode},
		{"Closing balance", money.Format(closing) + " " + coinCode},
	}
	for _, row := range rows {
		pdf.CellFormat(60, 6, row[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, row[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)
}

// writeChart embeds a chart PNG under a title, drawing a color-key legend
// first when the chart has more than one series (legend may be nil).
func writeChart(pdf *fpdf.Fpdf, title string, png []byte, legend [][2]string) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	if legend != nil {
		writeLegend(pdf, legend)
	}
	name := title + fmt.Sprint(pdf.PageNo())
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))
	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	w := pageW - left - right
	pdf.ImageOptions(name, -1, -1, w, 0, true, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.Ln(4)
}

func writeTransactions(pdf *fpdf.Fpdf, txs []domain.Transaction) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Transactions", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	widths := []float64{25, 90, 35, 30}
	headers := []string{"Date", "Description", "Amount", "Status"}
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	if len(txs) == 0 {
		pdf.CellFormat(180, 7, "No settled activity this period.", "1", 1, "L", false, 0, "")
		return
	}
	for _, t := range txs {
		when := t.CreatedAt
		if t.EffectiveAt != nil {
			when = *t.EffectiveAt
		}
		sign := "+"
		if t.Type == domain.TxRedeem || t.Type == domain.TxAdjustDebit {
			sign = "-"
		}
		amount := sign + money.Format(t.AmountMinor)
		pdf.CellFormat(widths[0], 6, when.Format("2006-01-02"), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, truncate(txDescription(t), 55), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 6, amount, "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], 6, string(t.Status), "1", 1, "L", false, 0, "")
	}
}

func txDescription(t domain.Transaction) string {
	labels := map[domain.TxType]string{
		domain.TxEarn: "Earn", domain.TxRedeem: "Redeem",
		domain.TxAdjustCredit: "Credit", domain.TxAdjustDebit: "Debit",
	}
	if t.Memo != "" {
		return labels[t.Type] + " - " + t.Memo
	}
	return labels[t.Type]
}

// truncate is deliberately ASCII-only ("..." not "…"): fpdf's core fonts use
// cp1252, and a raw UTF-8 multi-byte rune passed through renders as mojibake
// (e.g. em-dash showing up as "â€”") instead of the intended character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

func balanceChartPNG(points []domain.BalancePoint) ([]byte, error) {
	xs := make([]time.Time, len(points))
	ys := make([]float64, len(points))
	for i, p := range points {
		xs[i] = p.Bucket
		ys[i] = float64(p.BalanceMinor) / float64(money.MinorPerCoin)
	}
	c := chart.Chart{
		Width: chartWidth, Height: chartHeight, DPI: chartDPI,
		Background: chart.Style{Padding: chartPadding},
		XAxis:      chart.XAxis{Style: chart.Style{StrokeColor: colorAxis, FontColor: colorAxis}},
		YAxis:      chart.YAxis{Style: chart.Style{StrokeColor: colorAxis, FontColor: colorAxis}},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: xs, YValues: ys,
				Style: chart.Style{StrokeColor: colorLine, StrokeWidth: chartStrokeW, FillColor: colorLine.WithAlpha(40)},
			},
		},
	}
	var buf bytes.Buffer
	if err := c.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// earnRedeemChartPNG renders the two series as plain colored segments with no
// per-value Label: go-chart draws a non-empty Value.Label as free-floating
// text pinned to that segment regardless of available space, which on a
// chart with several narrow weekly bars produces overlapping "Earned"/
// "Redeemed" text on every bar. Color alone (matching the app's pos/neg
// convention) plus the manual legend drawn in writeLegend carries the
// identity instead.
func earnRedeemChartPNG(buckets []domain.EarnRedeemBucket) ([]byte, error) {
	bars := make([]chart.StackedBar, len(buckets))
	for i, b := range buckets {
		bars[i] = chart.StackedBar{
			Name: b.Bucket.Format("Jan 2"),
			Values: []chart.Value{
				{Value: float64(b.EarnedMinor) / float64(money.MinorPerCoin), Style: chart.Style{FillColor: colorCredit}},
				{Value: float64(b.RedeemedMinor) / float64(money.MinorPerCoin), Style: chart.Style{FillColor: colorDebit}},
			},
		}
	}
	c := chart.StackedBarChart{
		Width: chartWidth, Height: chartHeight, DPI: chartDPI,
		Background: chart.Style{Padding: chartPadding},
		Bars:       bars,
	}
	var buf bytes.Buffer
	if err := c.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeLegend draws a small color-key row (matching the web app's chart
// legends) above a chart that has more than one series.
func writeLegend(pdf *fpdf.Fpdf, items [][2]string) {
	// items are {label, hex color}
	x, y := pdf.GetXY()
	for _, item := range items {
		var r, g, b int
		fmt.Sscanf(item[1], "%02x%02x%02x", &r, &g, &b)
		pdf.SetFillColor(r, g, b)
		pdf.Rect(x, y+1.2, 3, 3, "F")
		pdf.SetXY(x+4.5, y)
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(24, 5, item[0], "", 0, "L", false, 0, "")
		x = pdf.GetX()
	}
	pdf.Ln(7)
}

// writeLeaderboard draws a native, vector ranked-bar list instead of a
// rasterized chart: go-chart's BarChart lays out one narrow column per bar
// with the category name wrapped underneath, which clips or mangles longer
// chore names (e.g. "Mow the lawn" losing "lawn"). A plain fpdf row — name,
// proportional bar, value — has no such layout constraint and stays crisp at
// any size since it's vector, not a raster embed.
func writeLeaderboard(pdf *fpdf.Fpdf, title string, entries []domain.TaskLeaderboardEntry) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")

	sorted := append([]domain.TaskLeaderboardEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TotalMinor > sorted[j].TotalMinor })
	shown := sorted
	if len(shown) > 5 {
		shown = shown[:5]
	}

	maxVal := int64(1)
	for _, e := range shown {
		if e.TotalMinor > maxVal {
			maxVal = e.TotalMinor
		}
	}

	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	const nameWidth, valueWidth = 55.0, 24.0
	barMaxWidth := pageW - left - right - nameWidth - valueWidth

	pdf.SetFont("Helvetica", "", 9)
	for _, e := range shown {
		name := e.TaskName
		if e.IsBounty {
			name += " (bounty)"
		}
		_, y := pdf.GetXY()
		pdf.CellFormat(nameWidth, 7, truncate(name, 32), "", 0, "L", false, 0, "")

		barWidth := barMaxWidth * float64(e.TotalMinor) / float64(maxVal)
		if barWidth < 1.5 {
			barWidth = 1.5
		}
		pdf.SetFillColor(14, 124, 102) // matches the app's --brand teal
		pdf.Rect(pdf.GetX(), y+1.3, barWidth, 4.4, "F")

		pdf.SetXY(pdf.GetX()+barMaxWidth+2, y)
		pdf.CellFormat(valueWidth, 7, money.Format(e.TotalMinor)+" "+money.Currency, "", 1, "R", false, 0, "")
	}
	if len(sorted) > len(shown) {
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 6, fmt.Sprintf("Showing top %d of %d chores by total earned.", len(shown), len(sorted)), "", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.Ln(4)
}
