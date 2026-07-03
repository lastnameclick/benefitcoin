package api

import (
	"fmt"
	"net/http"
	"time"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/statement"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
)

// parseChartRange reads optional from/to (YYYY-MM-DD) and bucket query params,
// defaulting to the trailing `defaultMonths` months.
func parseChartRange(r *http.Request, defaultMonths int, defaultBucket string) (from, to time.Time, bucket string, err error) {
	now := time.Now().UTC()
	to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	from = to.AddDate(0, -defaultMonths, 0)
	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.Parse("2006-01-02", v); err != nil {
			return
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.Parse("2006-01-02", v); err != nil {
			return
		}
	}
	bucket = r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = defaultBucket
	}
	return
}

func (s *Server) handleBalanceHistory(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	from, to, bucket, err := parseChartRange(r, 6, "day")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid from/to date")
		return
	}
	points, err := s.store.BalanceHistory(r.Context(), claims.TenantID, acct.ID, from, to, bucket)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load balance history")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": points})
}

func (s *Server) handleEarnRedeem(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	from, to, bucket, err := parseChartRange(r, 6, "week")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid from/to date")
		return
	}
	buckets, err := s.store.EarnRedeemSummary(r.Context(), claims.TenantID, acct.ID, from, to, bucket)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load earn/redeem summary")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets})
}

func (s *Server) handleRedemptionFrequency(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	from, to, _, err := parseChartRange(r, 6, "day")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid from/to date")
		return
	}
	freq, err := s.store.RedemptionFrequency(r.Context(), claims.TenantID, acct.ID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load redemption frequency")
		return
	}
	writeJSON(w, http.StatusOK, freq)
}

func (s *Server) handleTaskLeaderboard(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	from, to, _, err := parseChartRange(r, 6, "day")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid from/to date")
		return
	}
	entries, err := s.store.TaskLeaderboard(r.Context(), claims.TenantID, &acct.ID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load task leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// handleHouseholdLeaderboard is the operator's tenant-wide view (every
// holder's earnings pooled together).
func (s *Server) handleHouseholdLeaderboard(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	from, to, _, err := parseChartRange(r, 6, "day")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid from/to date")
		return
	}
	entries, err := s.store.TaskLeaderboard(r.Context(), claims.TenantID, nil, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load task leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

type holderSummaryView struct {
	AccountID      string `json:"account_id"`
	CustomerID     string `json:"customer_id"`
	DisplayName    string `json:"display_name"`
	CurrentMinor   int64  `json:"current_minor"`
	AvailableMinor int64  `json:"available_minor"`
	RecentTxCount  int64  `json:"recent_tx_count"`
}

// handleHouseholdOverview compares every holder's balance and recent
// activity side by side, for the operator console.
func (s *Server) handleHouseholdOverview(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	accts, err := s.store.ListCustomerAccounts(r.Context(), claims.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list accounts")
		return
	}
	custs, err := s.store.ListCustomers(r.Context(), claims.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list customers")
		return
	}
	custNames := make(map[string]string, len(custs))
	for _, c := range custs {
		custNames[c.ID] = c.DisplayName
	}
	counts, err := s.store.CountTransactionsSince(r.Context(), claims.TenantID, time.Now().UTC().AddDate(0, 0, -30))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load recent activity")
		return
	}

	views := make([]holderSummaryView, 0, len(accts))
	for _, a := range accts {
		bal, err := s.ledger.Balance(a.TBAccountID)
		if err != nil {
			continue
		}
		v := holderSummaryView{
			AccountID: a.ID, CurrentMinor: bal.Current(), AvailableMinor: bal.Available(),
			RecentTxCount: counts[a.ID], DisplayName: a.Name,
		}
		if a.CustomerID != nil {
			v.CustomerID = *a.CustomerID
			if name, ok := custNames[*a.CustomerID]; ok {
				v.DisplayName = name
			}
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"holders": views})
}

// holderDisplayName resolves a friendly name for the statement header.
func (s *Server) holderDisplayName(r *http.Request, tenantID string, acct domain.Account) string {
	if acct.CustomerID == nil {
		return acct.Name
	}
	cust, err := s.store.GetCustomer(r.Context(), tenantID, *acct.CustomerID)
	if err != nil {
		return acct.Name
	}
	return cust.DisplayName
}

// handleDownloadStatement generates a statement PDF on demand for the given
// (or current) month and streams it straight back. This is a one-off
// preview/reprint tool — unlike the monthly job, it does NOT save to the
// Inbox, so repeatedly generating the same month doesn't pile up duplicate
// entries there. The Inbox only ever reflects the job's official run.
func (s *Server) handleDownloadStatement(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	period := time.Now().UTC()
	if v := r.URL.Query().Get("period"); v != "" {
		parsed, err := time.Parse("2006-01", v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "period must be YYYY-MM")
			return
		}
		period = parsed
	}

	pdf, err := statement.Generate(r.Context(), statement.Deps{Store: s.store, Cfg: s.cfg}, tenant, acct,
		s.holderDisplayName(r, claims.TenantID, acct), period)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to generate statement: "+err.Error())
		return
	}
	writePDF(w, pdf, fmt.Sprintf("statement-%s.pdf", period.Format("2006-01")))
}

func (s *Server) handleListInbox(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	metas, err := s.store.ListStatements(r.Context(), claims.TenantID, acct.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list statements")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"statements": metas})
}

func (s *Server) handleDownloadInboxStatement(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "statementId")
	meta, pdf, err := s.store.GetStatementPDF(r.Context(), claims.TenantID, id)
	if err == store.ErrNotFound || (err == nil && meta.AccountID != acct.ID) {
		writeErr(w, http.StatusNotFound, "not_found", "statement not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load statement")
		return
	}
	_ = s.store.MarkStatementViewed(r.Context(), claims.TenantID, id)
	writePDF(w, pdf, fmt.Sprintf("statement-%s.pdf", meta.Period.Format("2006-01")))
}

func writePDF(w http.ResponseWriter, pdf []byte, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}
