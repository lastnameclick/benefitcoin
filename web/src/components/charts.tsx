import { useCallback, useEffect, useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  LabelList,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  api,
  ApiError,
  coins,
  type BalancePoint,
  type EarnRedeemBucket,
  type HolderSummary,
  type RedemptionFrequency,
  type StatementMeta,
  type TaskLeaderboardEntry,
} from "../api";
import {
  coinValue,
  fillRange,
  formatDay,
  formatMonthYear,
  hourLabel,
  monthLabel,
  recentPeriods,
  weekdayLabel,
} from "../lib/charts";
import { Empty, Money, Notice, Panel } from "./ui";
import { IconArrowRight } from "./icons";

function Loading() {
  return <div className="chart-loading">Loading…</div>;
}

// CoinTooltip renders a bucket's series as bolded coin values with a
// secondary label and a line-key swatch — never color-coded text.
function CoinTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null;
  return (
    <div className="chart-tip">
      {label && <div className="chart-tip-head">{label}</div>}
      {payload.map((p: any) => (
        <div className="chart-tip-row" key={p.dataKey}>
          <span className="chart-tip-key" style={{ background: p.color }} />
          <span className="chart-tip-label">{p.name}</span>
          <span className="chart-tip-value">
            <Money minor={Math.round(p.value * 1000)} signed />
          </span>
        </div>
      ))}
    </div>
  );
}

function CountTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null;
  const p = payload[0];
  return (
    <div className="chart-tip">
      {label && <div className="chart-tip-head">{label}</div>}
      <div className="chart-tip-row">
        <span className="chart-tip-key" style={{ background: p.color }} />
        <span className="chart-tip-label">Redemptions</span>
        <span className="chart-tip-value">{p.value}</span>
      </div>
    </div>
  );
}

// BalanceHistoryChart is a single-series trend — sequential brand hue, no
// legend needed (the panel title already names the one thing plotted).
export function BalanceHistoryChart({ points }: { points: BalancePoint[] }) {
  if (points.length < 2) {
    return <Empty title="Not enough history yet." hint="Check back after a few settled transactions." />;
  }
  const data = points.map((p) => ({ x: formatDay(p.bucket), v: coinValue(p.balance_minor) }));
  return (
    <div className="chart-box">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="balanceFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--brand)" stopOpacity={0.14} />
              <stop offset="100%" stopColor="var(--brand)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke="var(--line)" />
          <XAxis
            dataKey="x" tick={{ fill: "var(--muted)", fontSize: 11 }}
            axisLine={{ stroke: "var(--line-2)" }} tickLine={false} minTickGap={28}
          />
          <YAxis tick={{ fill: "var(--muted)", fontSize: 11 }} axisLine={false} tickLine={false} width={44} />
          <Tooltip content={<CoinTooltip />} />
          <Area
            type="monotone" dataKey="v" name="Balance" stroke="var(--brand)" strokeWidth={2}
            fill="url(#balanceFill)" dot={false}
            activeDot={{ r: 4, fill: "var(--brand)", stroke: "var(--surface)", strokeWidth: 2 }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

// EarnRedeemChart is a diverging bar per period — earned above the zero
// baseline, redeemed below, echoing the app's existing pos/neg convention
// (the same colors Money and TransactionsPanel already use for credits/debits).
export function EarnRedeemChart({ buckets }: { buckets: EarnRedeemBucket[] }) {
  if (buckets.length === 0) return <Empty title="No activity in this period." />;
  const data = buckets.map((b) => ({
    x: formatDay(b.bucket),
    earned: coinValue(b.earned_minor),
    redeemed: -coinValue(b.redeemed_minor),
  }));
  return (
    <div>
      <div className="chart-legend">
        <span className="chart-legend-item">
          <span className="chart-legend-swatch" style={{ background: "var(--pos)" }} /> Earned
        </span>
        <span className="chart-legend-item">
          <span className="chart-legend-swatch" style={{ background: "var(--neg)" }} /> Redeemed
        </span>
      </div>
      <div className="chart-box">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 16 }} stackOffset="sign">
            <CartesianGrid vertical={false} stroke="var(--line)" />
            <XAxis
              dataKey="x" tick={{ fill: "var(--muted)", fontSize: 11 }}
              axisLine={{ stroke: "var(--line-2)" }} tickLine={false}
            />
            <YAxis
              tick={{ fill: "var(--muted)", fontSize: 11 }} axisLine={false} tickLine={false} width={44}
              domain={[(min: number) => (min < 0 ? min * 1.2 : min), (max: number) => (max > 0 ? max * 1.2 : max)]}
            />
            <Tooltip content={<CoinTooltip />} />
            <Bar dataKey="earned" name="Earned" stackId="earnRedeem" fill="var(--pos)" radius={[4, 4, 0, 0]} maxBarSize={22} />
            <Bar dataKey="redeemed" name="Redeemed" stackId="earnRedeem" fill="var(--neg)" radius={[4, 4, 0, 0]} maxBarSize={22} />
            <ReferenceLine y={0} stroke="black" strokeWidth={2} isFront />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

// TaskLeaderboardChart is a horizontal bar (long chore names read better as
// rows than columns) — a single sequential hue, sorted, direct-labeled at
// the bar tip since there's only one series.
export function TaskLeaderboardChart({ entries }: { entries: TaskLeaderboardEntry[] }) {
  if (entries.length === 0) return <Empty title="No settled earnings yet." />;
  const sorted = [...entries].sort((a, b) => b.total_minor - a.total_minor);
  const shown = sorted.slice(0, 8);
  const data = shown.map((e) => ({
    name: e.is_bounty ? `${e.task_name} ★` : e.task_name,
    v: coinValue(e.total_minor),
  }));
  return (
    <div>
      <div className="chart-box" style={{ height: Math.max(140, 36 * data.length + 16) }}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} layout="vertical" margin={{ top: 4, right: 44, left: 0, bottom: 4 }}>
            <CartesianGrid horizontal={false} stroke="var(--line)" />
            <XAxis type="number" tick={{ fill: "var(--muted)", fontSize: 11 }} axisLine={false} tickLine={false} />
            <YAxis
              type="category" dataKey="name" width={140}
              tick={{ fill: "var(--text)", fontSize: 12 }} axisLine={false} tickLine={false}
            />
            <Tooltip content={<CoinTooltip />} />
            <Bar dataKey="v" name="Earned" fill="var(--brand)" radius={[0, 4, 4, 0]} maxBarSize={20}>
              <LabelList
                dataKey="v" position="right" fill="var(--muted)" fontSize={11}
                formatter={(v: number) => coins(Math.round(v * 1000))}
              />
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      {sorted.length > 8 && <p className="chart-note">Showing top 8 of {sorted.length} chores by total earned.</p>}
    </div>
  );
}

// RedemptionFrequencyCharts is three small multiples (same job — count by
// time bucket — repeated across three dimensions), each single-hue.
export function RedemptionFrequencyCharts({ freq }: { freq: RedemptionFrequency }) {
  const hasAny = [freq.by_hour, freq.by_weekday, freq.by_month].some((b) => b.some((x) => x.count > 0));
  if (!hasAny) return <Empty title="No redemptions in this period." />;
  const hourData = fillRange(freq.by_hour, 24).map((b) => ({ x: hourLabel(b.bucket), v: b.count }));
  const weekdayData = fillRange(freq.by_weekday, 7).map((b) => ({ x: weekdayLabel(b.bucket), v: b.count }));
  const monthData = fillRange(freq.by_month, 12, 1).map((b) => ({ x: monthLabel(b.bucket), v: b.count }));
  return (
    <div className="chart-stack">
      <FrequencyMini title="By time of day" data={hourData} />
      <div className="chart-grid-2">
        <FrequencyMini title="By day of week" data={weekdayData} />
        <FrequencyMini title="By month" data={monthData} />
      </div>
    </div>
  );
}

function FrequencyMini({ title, data }: { title: string; data: { x: string; v: number }[] }) {
  return (
    <div className="chart-mini-box">
      <div className="chart-mini-title">{title}</div>
      <div className="chart-box chart-box-sm">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
            <XAxis
              dataKey="x" tick={{ fill: "var(--muted)", fontSize: 9.5 }}
              axisLine={false} tickLine={false} interval={0}
            />
            <YAxis hide allowDecimals={false} />
            <Tooltip content={<CountTooltip />} />
            <Bar dataKey="v" name="Redemptions" fill="var(--brand)" radius={[3, 3, 0, 0]} maxBarSize={16} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

// HouseholdOverviewChart compares holder balances (one measure, one chart);
// recent activity is a second measure on a different scale, so it lives in a
// plain column instead of a second axis on the same chart.
export function HouseholdOverviewChart({ holders }: { holders: HolderSummary[] }) {
  if (holders.length === 0) return <Empty title="No holder accounts yet." />;
  const sorted = [...holders].sort((a, b) => b.current_minor - a.current_minor);
  const data = sorted.map((h) => ({ name: h.display_name, v: coinValue(h.current_minor) }));
  return (
    <div className="grid-2">
      <div className="span-2 chart-box" style={{ height: Math.max(160, 40 * data.length + 16) }}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} layout="vertical" margin={{ top: 4, right: 44, left: 0, bottom: 4 }}>
            <CartesianGrid horizontal={false} stroke="var(--line)" />
            <XAxis type="number" tick={{ fill: "var(--muted)", fontSize: 11 }} axisLine={false} tickLine={false} />
            <YAxis
              type="category" dataKey="name" width={120}
              tick={{ fill: "var(--text)", fontSize: 12 }} axisLine={false} tickLine={false}
            />
            <Tooltip content={<CoinTooltip />} />
            <Bar dataKey="v" name="Ledger balance" fill="var(--brand)" radius={[0, 4, 4, 0]} maxBarSize={22}>
              <LabelList
                dataKey="v" position="right" fill="var(--muted)" fontSize={11}
                formatter={(v: number) => coins(Math.round(v * 1000))}
              />
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <table className="data-table span-2">
        <thead>
          <tr>
            <th>Holder</th>
            <th className="amt">Balance</th>
            <th className="amt">Activity (30d)</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((h) => (
            <tr key={h.account_id}>
              <td>{h.display_name}</td>
              <td className="amt"><Money minor={h.current_minor} /></td>
              <td className="amt">{h.recent_tx_count}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// AccountCharts fetches and renders the four per-holder charts — used by both
// the holder's own "Charts" section and the operator's per-holder drill-down.
export function AccountCharts({ accountId }: { accountId: string }) {
  const [balance, setBalance] = useState<BalancePoint[] | null>(null);
  const [earnRedeem, setEarnRedeem] = useState<EarnRedeemBucket[] | null>(null);
  const [leaderboard, setLeaderboard] = useState<TaskLeaderboardEntry[] | null>(null);
  const [freq, setFreq] = useState<RedemptionFrequency | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let alive = true;
    setBalance(null);
    setEarnRedeem(null);
    setLeaderboard(null);
    setFreq(null);
    Promise.all([
      api.balanceHistory(accountId),
      api.earnRedeemSummary(accountId),
      api.taskLeaderboard(accountId),
      api.redemptionFrequency(accountId),
    ])
      .then(([b, e, l, f]) => {
        if (!alive) return;
        setBalance(b.points ?? []);
        setEarnRedeem(e.buckets ?? []);
        setLeaderboard(l.entries ?? []);
        setFreq({
          by_hour: f.by_hour ?? [],
          by_weekday: f.by_weekday ?? [],
          by_month: f.by_month ?? [],
        });
      })
      .catch((e) => alive && setErr(e instanceof ApiError ? e.message : String(e)));
    return () => {
      alive = false;
    };
  }, [accountId]);

  return (
    <div className="chart-stack">
      <Notice err={err} />
      <Panel title="Balance over time" sub="Settled balance over the last 6 months.">
        {balance ? <BalanceHistoryChart points={balance} /> : <Loading />}
      </Panel>
      <Panel title="Earned vs. redeemed" sub="Weekly settled activity.">
        {earnRedeem ? <EarnRedeemChart buckets={earnRedeem} /> : <Loading />}
      </Panel>
      <Panel title="Top chores" sub="Ranked by total settled earnings.">
        {leaderboard ? <TaskLeaderboardChart entries={leaderboard} /> : <Loading />}
      </Panel>
      <div className="chart-divider" />
      <Panel title="When redemptions happen" sub="Settled redemptions by time.">
        {freq ? <RedemptionFrequencyCharts freq={freq} /> : <Loading />}
      </Panel>
    </div>
  );
}

// HouseholdCharts is the operator's tenant-wide view: every holder compared,
// plus the chore leaderboard pooled across the whole household.
export function HouseholdCharts() {
  const [overview, setOverview] = useState<HolderSummary[] | null>(null);
  const [leaderboard, setLeaderboard] = useState<TaskLeaderboardEntry[] | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let alive = true;
    Promise.all([api.householdOverview(), api.householdLeaderboard()])
      .then(([o, l]) => {
        if (!alive) return;
        setOverview(o.holders ?? []);
        setLeaderboard(l.entries ?? []);
      })
      .catch((e) => alive && setErr(e instanceof ApiError ? e.message : String(e)));
    return () => {
      alive = false;
    };
  }, []);

  return (
    <div className="grid-2">
      <Notice err={err} />
      <Panel className="span-2" title="Household balances" sub="Every holder, side by side.">
        {overview ? <HouseholdOverviewChart holders={overview} /> : <Loading />}
      </Panel>
      <Panel className="span-2" title="Top chores across the household" sub="Ranked by total settled earnings.">
        {leaderboard ? <TaskLeaderboardChart entries={leaderboard} /> : <Loading />}
      </Panel>
    </div>
  );
}

const STATEMENT_PERIODS = recentPeriods(24);

// Inbox lists every generated statement for an account and lets the holder
// (or operator) download any of them — the guaranteed delivery channel that
// works with zero SMTP configuration — plus a picker to generate any past
// month's statement on demand (not just the current one).
export function Inbox({ accountId }: { accountId: string }) {
  const [statements, setStatements] = useState<StatementMeta[]>([]);
  const [period, setPeriod] = useState(STATEMENT_PERIODS[0].value);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const { statements } = await api.listInbox(accountId);
    setStatements(statements ?? []);
  }, [accountId]);

  useEffect(() => {
    load().catch((e) => setErr(e instanceof ApiError ? e.message : String(e)));
  }, [load]);

  // refresh=true after an Inbox download (it may have just been marked
  // viewed); a manual "generate" isn't saved to the Inbox, so there's nothing
  // to refresh.
  const download = async (fn: () => Promise<void>, refresh = true) => {
    setErr("");
    setBusy(true);
    try {
      await fn();
      if (refresh) await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Panel
      title="Inbox"
      sub="Monthly statements, generated automatically and always available here — no email setup required."
    >
      <Notice err={err} />
      {statements.length === 0 ? (
        <Empty title="No statements yet." hint="One is generated automatically each month, or pick a month below to generate one now." />
      ) : (
        <ul className="rows">
          {statements.map((s) => (
            <li key={s.id} className="row">
              <div>
                <div className="row-title">
                  {formatMonthYear(s.period)}
                  {!s.viewed_at && <span className="chip chip-new">new</span>}
                </div>
                <div className="row-sub">
                  Generated {new Date(s.generated_at).toLocaleDateString()}
                  {s.emailed_at ? " · emailed" : ""}
                </div>
              </div>
              <button
                className="btn-ghost sm" disabled={busy}
                onClick={() => download(() => api.downloadInboxStatement(accountId, s.id, s.period.slice(0, 7)))}
              >
                <IconArrowRight width={15} height={15} /> Download
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="hint-line generate-statement" style={{ marginTop: 12 }}>
        <label className="field-inline">
          Generate a statement for
          <select value={period} onChange={(e) => setPeriod(e.target.value)} disabled={busy}>
            {STATEMENT_PERIODS.map((p) => (
              <option key={p.value} value={p.value}>{p.label}</option>
            ))}
          </select>
        </label>
        <button
          className="btn-primary sm" disabled={busy}
          onClick={() => download(() => api.downloadStatementPdf(accountId, period), false)}
        >
          Generate
        </button>
      </div>
      <p className="chart-note" style={{ marginTop: 4 }}>Downloads immediately — this doesn't get saved to your Inbox above.</p>
    </Panel>
  );
}
