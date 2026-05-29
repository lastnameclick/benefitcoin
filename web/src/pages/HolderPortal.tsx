import { useCallback, useEffect, useState } from "react";
import { api, ApiError, type Account, type Task, type Transaction } from "../api";
import { useBranding } from "../branding";
import { DashboardShell, type Section } from "../components/DashboardShell";
import { BalanceTiles, Empty, Info, Money, Notice, Panel, TransactionsPanel } from "../components/ui";
import Glossary from "../components/Glossary";
import { IconActivity, IconArrowRight, IconBook, IconCheck, IconHome, IconList } from "../components/icons";

export default function HolderPortal() {
  const [account, setAccount] = useState<Account | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [txs, setTxs] = useState<Transaction[]>([]);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    const [{ accounts }, { tasks }] = await Promise.all([api.listAccounts(), api.listTasks()]);
    const acct = accounts[0] ?? null;
    setAccount(acct);
    setTasks(tasks ?? []);
    if (acct) {
      const { transactions } = await api.accountTransactions(acct.id);
      setTxs(transactions ?? []);
    }
  }, []);

  useEffect(() => {
    load().catch((e) => setErr(String(e)));
  }, [load]);

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    setMsg("");
    setErr("");
    try {
      await fn();
      setMsg(ok);
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    }
  };

  const nameFor = () => account?.name ?? "Wallet";

  const sections: Section[] = [
    {
      key: "overview", label: "Overview", icon: <IconHome />,
      hint: account ? `Account No. ${account.id.slice(0, 8).toUpperCase()}` : "No account yet",
      render: () => (
        <Overview
          account={account} txs={txs} msg={msg} err={err} nameFor={nameFor}
          onRedeem={() => act(() => api.requestRedemption(account!.id), "Redemption request sent for approval.")}
        />
      ),
    },
    {
      key: "statement", label: "Statement", icon: <IconActivity />,
      render: () =>
        account ? (
          <Panel title="Statement" sub="Every posting on your account. Select a row to see the double-entry detail.">
            <TransactionsPanel txs={txs} accountNameFor={nameFor} />
          </Panel>
        ) : (
          <Panel title="Statement"><Empty title="No account yet." /></Panel>
        ),
    },
    {
      key: "earn", label: "Ways to earn", icon: <IconList />,
      render: () => (
        <Earn tasks={tasks} disabled={!account} msg={msg} err={err}
          onDone={(t) => act(() => api.submitEarning(account!.id, t.id), `Sent "${t.name}" for approval.`)} />
      ),
    },
    { key: "learn", label: "Learn", icon: <IconBook />, render: () => <Glossary /> },
  ];

  return <DashboardShell sections={sections} />;
}

function Overview({
  account, txs, msg, err, onRedeem, nameFor,
}: {
  account: Account | null; txs: Transaction[]; msg: string; err: string;
  onRedeem: () => void; nameFor: () => string;
}) {
  const b = useBranding();
  if (!account) return <Panel title="Welcome"><Empty title="No account on file yet." hint="Ask a parent to open one for you." /></Panel>;

  const bal = account.balance;
  const canRedeem = (bal?.available_minor ?? 0) >= 1000;

  return (
    <div className="grid-2">
      <Panel className="span-2" title={account.name}
        actions={<button className="btn-primary" disabled={!canRedeem} onClick={onRedeem}>
          {canRedeem ? <>Redeem a reward — 1.00 <IconArrowRight width={16} height={16} /></> : `Save 1 ${b.coin_name} to redeem`}
        </button>}>
        <BalanceTiles balance={bal} />
        <Notice msg={msg} err={err} />
      </Panel>

      <Panel className="span-2" title="How your money works" sub="This account behaves like a real bank account.">
        <ul className="explain-list">
          <li><span className="explain-k">Earn</span><span>Finish a chore to request coins. It becomes a <b>hold</b> — reserved, but not spendable until a parent approves it.</span></li>
          <li><span className="explain-k">Approve</span><span>When approved, the hold <b>settles</b> and your <b>ledger balance</b> goes up. <Info text="Ledger balance = settled money. Available balance = what you can spend right now." /></span></li>
          <li><span className="explain-k">Redeem</span><span>At a whole {b.coin_name}, spend it on a reward. The amount is <b>debited</b> from your wallet.</span></li>
        </ul>
      </Panel>

      <Panel className="span-2" title="Recent activity">
        <TransactionsPanel txs={txs.slice(0, 6)} accountNameFor={nameFor} />
      </Panel>
    </div>
  );
}

function Earn({
  tasks, disabled, onDone, msg, err,
}: { tasks: Task[]; disabled: boolean; onDone: (t: Task) => void; msg: string; err: string }) {
  const active = tasks.filter((t) => t.active);
  return (
    <Panel title="Ways to earn" sub="Mark a chore done to submit it for approval.">
      <Notice msg={msg} err={err} />
      {active.length === 0 ? (
        <Empty title="Nothing to earn yet." hint="A parent hasn't posted any chores." />
      ) : (
        <ul className="rows">
          {active.map((t) => (
            <li key={t.id} className="row">
              <div>
                <div className="row-title">{t.name}</div>
                {t.description && <div className="row-sub">{t.description}</div>}
              </div>
              <div className="row-right">
                <Money minor={t.value_minor} signed className="pos" />
                <button className="btn-ghost" disabled={disabled} onClick={() => onDone(t)}>
                  <IconCheck width={16} height={16} /> Mark done
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
