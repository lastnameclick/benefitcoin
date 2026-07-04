import { useState, type ReactNode } from "react";
import { coins, isCredit, txLabel, type Balance, type Transaction } from "../api";
import { useBranding } from "../branding";
import { lifecycle, postingLegs, txExplainer } from "../lib/txn";
import { IconInfo, IconX } from "./icons";

// Info is an accessible ⓘ affordance that reveals a short definition on hover or
// focus — the backbone of the "teach as you go" banking education.
export function Info({ text }: { text: string }) {
  return (
    <span className="info" tabIndex={0} role="note">
      <IconInfo width={14} height={14} />
      <span className="info-bubble">{text}</span>
    </span>
  );
}

// Money renders an amount in coin units with the operator's ticker.
export function Money({
  minor, signed = false, code = true, className = "",
}: { minor: number; signed?: boolean; code?: boolean; className?: string }) {
  const b = useBranding();
  const neg = minor < 0;
  const sign = neg ? "−" : signed ? "+" : "";
  return (
    <span className={`money ${className}`}>
      {sign}{coins(Math.abs(minor))}
      {code && <span className="ccy"> {b.coin_code}</span>}
    </span>
  );
}

export function StatusPill({ status }: { status: string }) {
  return <span className={`pill pill-${status}`}>{status}</span>;
}

// Panel is the standard white card with an optional titled header.
export function Panel({
  title, sub, actions, children, className = "",
}: { title?: string; sub?: string; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`panel ${className}`}>
      {(title || actions) && (
        <div className="panel-head">
          <div>
            {title && <h2>{title}</h2>}
            {sub && <p className="panel-sub">{sub}</p>}
          </div>
          {actions}
        </div>
      )}
      {children}
    </section>
  );
}

export function Notice({ msg, err }: { msg?: string; err?: string }) {
  if (err) return <div className="notice notice-err">{err}</div>;
  if (msg) return <div className="notice notice-ok">{msg}</div>;
  return null;
}

export function Empty({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="empty">
      <p className="empty-title">{title}</p>
      {hint && <p className="empty-hint">{hint}</p>}
    </div>
  );
}

// BalanceTiles is the account summary strip, with a definition on each balance —
// the distinction between available, ledger, and pending is core banking literacy.
export function BalanceTiles({ balance }: { balance?: Balance }) {
  const tiles = [
    {
      label: "Available balance", value: balance?.available_minor ?? 0, primary: true,
      info: "What can be spent right now: the settled balance minus any funds reserved by pending redemption holds.",
    },
    {
      label: "Ledger balance", value: balance?.current_minor ?? 0,
      info: "The settled balance — posted credits minus posted debits. Also called the current or book balance.",
    },
    {
      label: "Pending", value: balance?.awaiting_approval_minor ?? 0,
      info: "Earnings submitted but not yet approved. Held as a pending credit; not spendable until an operator settles them.",
    },
  ];
  return (
    <div className="tiles">
      {tiles.map((t) => (
        <div key={t.label} className={`tile ${t.primary ? "tile-primary" : ""}`}>
          <div className="tile-label">{t.label} <Info text={t.info} /></div>
          <div className="tile-value"><Money minor={t.value} /></div>
        </div>
      ))}
    </div>
  );
}

// Statement is a clickable transaction table; a row opens the posting drawer.
export function Statement({
  txs, accountName, onSelect,
}: { txs: Transaction[]; accountName?: (id: string) => string; onSelect: (t: Transaction) => void }) {
  if (txs.length === 0) return <Empty title="No transactions yet." hint="Postings will appear here as they happen." />;
  return (
    <div className="table-scroll">
      <table className="ledger-table">
        <thead>
          <tr>
            <th>Date</th>
            {accountName && <th>Account</th>}
            <th>Description</th>
            <th className="amt">Amount</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {txs.map((t) => {
            const credit = isCredit(t);
            const when = t.effective_at ?? t.created_at;
            const note = typeof t.details?.note === "string" ? (t.details.note as string) : "";
            return (
              <tr key={t.id} className="ledger-row" onClick={() => onSelect(t)} tabIndex={0}
                onKeyDown={(e) => e.key === "Enter" && onSelect(t)}>
                <td className="nowrap muted">{new Date(when).toLocaleDateString()}</td>
                {accountName && <td className="muted">{accountName(t.account_id)}</td>}
                <td>
                  <span className="entry-label">{txLabel(t)}</span>
                  <span className="entry-memo"> {t.memo}</span>
                  {note && <div className="entry-note">{note}</div>}
                </td>
                <td className={`amt ${credit ? "pos" : "neg"}`}><Money minor={t.amount_minor} signed /></td>
                <td><StatusPill status={t.status} /></td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// TransactionsPanel wires a statement to the posting drawer.
export function TransactionsPanel({
  txs, accountNameFor,
}: { txs: Transaction[]; accountNameFor?: (id: string) => string }) {
  const [selected, setSelected] = useState<Transaction | null>(null);
  return (
    <>
      <Statement txs={txs} accountName={accountNameFor} onSelect={setSelected} />
      {selected && (
        <PostingDrawer
          tx={selected}
          walletName={accountNameFor ? accountNameFor(selected.account_id) : "Wallet"}
          onClose={() => setSelected(null)}
        />
      )}
    </>
  );
}

// PostingDrawer is the educational centrepiece: it shows the double-entry legs,
// the authorization → settlement lifecycle, and the underlying ledger references.
export function PostingDrawer({
  tx, walletName, onClose,
}: { tx: Transaction; walletName: string; onClose: () => void }) {
  const legs = postingLegs(tx, walletName);
  const steps = lifecycle(tx);
  return (
    <div className="drawer-scrim" onClick={onClose}>
      <aside className="drawer" onClick={(e) => e.stopPropagation()}>
        <header className="drawer-head">
          <div>
            <div className="eyebrow">Transaction</div>
            <h3>{txLabel(tx)}</h3>
          </div>
          <button className="icon-btn" onClick={onClose} aria-label="Close"><IconX /></button>
        </header>

        <div className="drawer-amount">
          <Money minor={tx.amount_minor} signed={false} className="big" />
          <StatusPill status={tx.status} />
        </div>
        <p className="drawer-explain">{txExplainer(tx)}</p>

        <section className="drawer-block">
          <div className="block-title">Double-entry postings <Info text="Every movement debits one account and credits another for the same amount. The two sides always balance." /></div>
          <table className="legs">
            <tbody>
              {legs.map((l) => (
                <tr key={l.side}>
                  <td className={`leg-side leg-${l.side}`}>{l.side === "debit" ? "DR" : "CR"}</td>
                  <td className="leg-acct">{l.account}</td>
                  <td className="leg-amt"><Money minor={tx.amount_minor} code={false} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <section className="drawer-block">
          <div className="block-title">Lifecycle</div>
          <ol className="stepper">
            {steps.map((s) => (
              <li key={s.label} className={`step step-${s.state}`}>
                <span className="step-dot" />
                <div>
                  <div className="step-label">{s.label}</div>
                  <div className="step-hint">{s.hint}</div>
                </div>
              </li>
            ))}
          </ol>
        </section>

        <section className="drawer-block">
          <div className="block-title">Details</div>
          <dl className="facts">
            <Fact label="Memo" value={tx.memo || "—"} />
            {tx.effective_at && <Fact label="Value date" value={new Date(tx.effective_at).toLocaleDateString()} />}
            <Fact label="Booked" value={new Date(tx.created_at).toLocaleString()} />
            {tx.decided_at && <Fact label="Decided" value={new Date(tx.decided_at).toLocaleString()} />}
            <Fact label="Pending transfer" value={tx.tb_pending_transfer_id} mono />
            {tx.tb_post_transfer_id && <Fact label="Post transfer" value={tx.tb_post_transfer_id} mono />}
            <Fact label="Transaction ID" value={tx.id} mono />
          </dl>
        </section>
      </aside>
    </div>
  );
}

function Fact({ label, value, mono }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <div className="fact">
      <dt>{label}</dt>
      <dd className={mono ? "mono" : ""}>{value}</dd>
    </div>
  );
}
