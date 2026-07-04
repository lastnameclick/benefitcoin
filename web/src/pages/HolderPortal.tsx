import {
  useCallback,
  useEffect,
  useState,
  type FormEvent,
} from "react";
import {
  api,
  ApiError,
  type Account,
  type Task,
  type Transaction,
} from "../api";
import { useBranding } from "../branding";
import { AccountCharts, Inbox } from "../components/charts";
import { DashboardShell, type Section } from "../components/DashboardShell";
import { relativeTime } from "../lib/time";
import {
  BalanceTiles,
  Empty,
  Info,
  Money,
  Notice,
  Panel,
  TransactionsPanel,
} from "../components/ui";
import {
  IconActivity,
  IconArrowRight,
  IconChart,
  IconCheck,
  IconHome,
  IconInbox,
  IconList,
  IconZap,
} from "../components/icons";

export default function HolderPortal() {
  const [account, setAccount] = useState<Account | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [txs, setTxs] = useState<Transaction[]>([]);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  const bountyCount = tasks.filter((t) => t.is_bounty && t.active && !t.claimed_by).length;

  const load = useCallback(async () => {
    const [{ accounts }, { tasks }] = await Promise.all([
      api.listAccounts(),
      api.listTasks(),
    ]);
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
      key: "overview",
      label: "Overview",
      icon: <IconHome />,
      hint: account
        ? `Account No. ${account.id.slice(0, 8).toUpperCase()}`
        : "No account yet",
      render: () => (
        <Overview
          account={account}
          txs={txs}
          msg={msg}
          err={err}
          nameFor={nameFor}
          onRedeem={() =>
            act(
              () => api.requestRedemption(account!.id),
              "Redemption request sent for approval.",
            )
          }
        />
      ),
    },
    {
      key: "statement",
      label: "Statement",
      icon: <IconActivity />,
      render: () =>
        account ? (
          <Panel
            title="Statement"
            sub="Every posting on your account. Select a row to see the double-entry detail."
          >
            <TransactionsPanel txs={txs} accountNameFor={nameFor} />
          </Panel>
        ) : (
          <Panel title="Statement">
            <Empty title="No account yet." />
          </Panel>
        ),
    },
    {
      key: "earn",
      label: "Ways to earn",
      icon: <IconList />,
      badge: bountyCount || undefined,
      hint: bountyCount
        ? `${bountyCount} limited-time ${bountyCount > 1 ? "bounties" : "bounty"} up for grabs`
        : undefined,
      render: () => (
        <Earn
          tasks={tasks}
          disabled={!account}
          msg={msg}
          err={err}
          onDone={(t) =>
            act(
              () => api.submitEarning(account!.id, t.id),
              `Sent "${t.name}" for approval.`,
            )
          }
          onPropose={(description, value) =>
            act(
              () => api.proposeChore(account!.id, description, value),
              "Sent your chore for approval.",
            )
          }
        />
      ),
    },
    {
      key: "charts",
      label: "Charts",
      icon: <IconChart />,
      render: () =>
        account ? (
          <AccountCharts accountId={account.id} />
        ) : (
          <Panel title="Charts">
            <Empty title="No account yet." />
          </Panel>
        ),
    },
    {
      key: "inbox",
      label: "Inbox",
      icon: <IconInbox />,
      render: () =>
        account ? (
          <Inbox accountId={account.id} />
        ) : (
          <Panel title="Inbox">
            <Empty title="No account yet." />
          </Panel>
        ),
    },
  ];

  return <DashboardShell sections={sections} />;
}

function Overview({
  account,
  txs,
  msg,
  err,
  onRedeem,
  nameFor,
}: {
  account: Account | null;
  txs: Transaction[];
  msg: string;
  err: string;
  onRedeem: () => void;
  nameFor: () => string;
}) {
  const b = useBranding();
  if (!account)
    return (
      <Panel title="Welcome">
        <Empty
          title="No account on file yet."
          hint="Ask a parent to open one for you."
        />
      </Panel>
    );

  const bal = account.balance;
  const canRedeem = (bal?.available_minor ?? 0) >= 1000;

  return (
    <div className="grid-2">
      <Panel
        className="span-2"
        title={account.name}
        actions={
          <button
            className="btn-primary"
            disabled={!canRedeem}
            onClick={onRedeem}
          >
            {canRedeem ? (
              <>
                Redeem a reward — 1.00 <IconArrowRight width={16} height={16} />
              </>
            ) : (
              `Save 1 ${b.coin_name} to redeem`
            )}
          </button>
        }
      >
        <BalanceTiles balance={bal} />
        <Notice msg={msg} err={err} />
      </Panel>

      <Panel
        className="span-2"
        title="How your money works"
        sub="This account behaves like a real bank account."
      >
        <ul className="explain-list">
          <li>
            <span className="explain-k">Earn</span>
            <span>
              Finish a chore to request coins. It becomes a <b>hold</b> —
              reserved, but not spendable until a parent approves it.
            </span>
          </li>
          <li>
            <span className="explain-k">Approve</span>
            <span>
              When approved, the hold <b>settles</b> and your{" "}
              <b>ledger balance</b> goes up.{" "}
              <Info text="Ledger balance = settled money. Available balance = what you can spend right now." />
            </span>
          </li>
          <li>
            <span className="explain-k">Redeem</span>
            <span>
              At a whole {b.coin_name}, spend it on a reward. The amount is{" "}
              <b>debited</b> from your wallet.
            </span>
          </li>
        </ul>
      </Panel>

      <Panel className="span-2" title="Recent activity">
        <TransactionsPanel txs={txs.slice(0, 6)} accountNameFor={nameFor} />
      </Panel>
    </div>
  );
}

function Earn({
  tasks,
  disabled,
  onDone,
  onPropose,
  msg,
  err,
}: {
  tasks: Task[];
  disabled: boolean;
  onDone: (t: Task) => void;
  onPropose: (description: string, value: string) => void;
  msg: string;
  err: string;
}) {
  const active = tasks.filter((t) => t.active && !t.is_bounty);
  const bounties = tasks.filter((t) => t.is_bounty && t.active && !t.claimed_by);

  return (
    <div className="grid-2">
      {bounties.length > 0 && (
        <Panel className="span-2 bounty-panel" title="Limited-time bounties">
          <ul className="rows">
            {bounties.map((t) => (
              <li key={t.id} className="row">
                <div>
                  <div className="row-title">
                    <IconZap width={15} height={15} className="bounty-icon" /> {t.name}{" "}
                    <span className="chip chip-bounty">one-time bounty</span>
                  </div>
                  {t.description && (
                    <div className="row-sub">{t.description}</div>
                  )}
                  {t.expires_at && (
                    <div className="row-sub bounty-deadline">
                      Ends {new Date(t.expires_at).toLocaleString()} (
                      {relativeTime(t.expires_at)})
                    </div>
                  )}
                </div>
                <div className="row-right">
                  <Money minor={t.value_minor} signed className="pos" />
                  <button
                    className="btn-primary"
                    disabled={disabled}
                    onClick={() => onDone(t)}
                  >
                    <IconCheck width={16} height={16} /> Claim it
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
      )}

      <Panel
        className="span-2"
        title="Ways to earn"
        sub="Mark a chore done to submit it for approval."
      >
        <Notice msg={msg} err={err} />
        {active.length === 0 ? (
          <Empty
            title="Nothing to earn yet."
            hint="A parent hasn't posted any chores."
          />
        ) : (
          <ul className="rows">
            {active.map((t) => (
              <li key={t.id} className="row">
                <div>
                  <div className="row-title">{t.name}</div>
                  {t.description && (
                    <div className="row-sub">{t.description}</div>
                  )}
                </div>
                <div className="row-right">
                  <Money minor={t.value_minor} signed className="pos" />
                  <button
                    className="btn-ghost"
                    disabled={disabled}
                    onClick={() => onDone(t)}
                  >
                    <IconCheck width={16} height={16} /> Mark done
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <ProposeChore disabled={disabled} onPropose={onPropose} />
    </div>
  );
}

function ProposeChore({
  disabled,
  onPropose,
}: {
  disabled: boolean;
  onPropose: (description: string, value: string) => void;
}) {
  const b = useBranding();
  const [description, setDescription] = useState("");
  const [value, setValue] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    onPropose(description, value);
    setDescription("");
    setValue("");
  };

  return (
    <Panel
      className="span-2"
      title="Did something that's not on the list?"
      sub="Describe it and propose what it's worth. A parent can approve, decline, or change the amount."
    >
      <form onSubmit={submit} className="form">
        <label className="field">
          What did you do?
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Organized the garage"
            required
          />
        </label>
        <label className="field">
          Proposed reward ({b.coin_name_plural}, e.g. 0.25)
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="0.25"
            required
          />
        </label>
        <button className="btn-primary" disabled={disabled}>
          Send for approval
        </button>
      </form>
    </Panel>
  );
}
