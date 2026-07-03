import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  api,
  ApiError,
  coins,
  isCredit,
  type Account,
  type AuditEvent,
  type Customer,
  type Task,
  type Transaction,
} from "../api";
import { useBranding } from "../branding";
import { AccountCharts, HouseholdCharts, Inbox } from "../components/charts";
import { DashboardShell, type Section } from "../components/DashboardShell";
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
  IconAdjust,
  IconArrowRight,
  IconChart,
  IconCheck,
  IconHome,
  IconInbox,
  IconList,
  IconShield,
  IconUsers,
  IconX,
  IconZap,
} from "../components/icons";

export default function OperatorConsole() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [pending, setPending] = useState(0);

  const loadShell = useCallback(async () => {
    const [{ accounts }, { transactions }] = await Promise.all([
      api.listAccounts(),
      api.pendingTransactions(),
    ]);
    setAccounts(accounts ?? []);
    setPending((transactions ?? []).length);
  }, []);
  useEffect(() => {
    loadShell().catch(() => {});
  }, [loadShell]);

  const nameFor = useCallback(
    (id: string) => accounts.find((a) => a.id === id)?.name ?? "Account",
    [accounts],
  );

  const sections: Section[] = [
    {
      key: "overview",
      label: "Overview",
      icon: <IconHome />,
      hint: "Household position at a glance",
      render: () => <Overview accounts={accounts} nameFor={nameFor} />,
    },
    {
      key: "approvals",
      label: "Approvals",
      icon: <IconInbox />,
      badge: pending || undefined,
      hint: "Authorization holds waiting to settle",
      render: () => <Approvals nameFor={nameFor} onChange={loadShell} />,
    },
    {
      key: "accounts",
      label: "Accounts",
      icon: <IconUsers />,
      hint: "Account holders and their wallets",
      render: () => <Accounts onChange={loadShell} />,
    },
    {
      key: "activity",
      label: "Activity",
      icon: <IconActivity />,
      hint: "All postings across the household",
      render: () => <Activity nameFor={nameFor} />,
    },
    {
      key: "charts",
      label: "Charts",
      icon: <IconChart />,
      hint: "Household-wide trends and comparisons",
      render: () => <HouseholdCharts />,
    },
    {
      key: "adjust",
      label: "Adjust",
      icon: <IconAdjust />,
      hint: "Post a manual journal entry",
      render: () => <Adjust onChange={loadShell} />,
    },
    {
      key: "chores",
      label: "Chores",
      icon: <IconList />,
      hint: "The earning catalog",
      render: () => <Chores />,
    },
    {
      key: "audit",
      label: "Audit",
      icon: <IconShield />,
      hint: "Append-only record of every change",
      render: () => <Audit />,
    },
  ];

  return <DashboardShell sections={sections} />;
}

function useError() {
  const [err, setErr] = useState("");
  const run = async (fn: () => Promise<unknown>, after?: () => void) => {
    setErr("");
    try {
      await fn();
      after?.();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    }
  };
  return { err, run };
}

function Overview({
  accounts,
  nameFor,
}: {
  accounts: Account[];
  nameFor: (id: string) => string;
}) {
  const b = useBranding();
  const [recent, setRecent] = useState<Transaction[]>([]);
  useEffect(() => {
    api
      .listTransactions()
      .then(({ transactions }) => setRecent(transactions ?? []))
      .catch(() => {});
  }, []);

  const totals = useMemo(() => {
    const sum = (f: (a: Account) => number) =>
      accounts.reduce((n, a) => n + f(a), 0);
    return {
      ledger: sum((a) => a.balance?.current_minor ?? 0),
      available: sum((a) => a.balance?.available_minor ?? 0),
      pending: sum((a) => a.balance?.awaiting_approval_minor ?? 0),
    };
  }, [accounts]);

  return (
    <div className="grid-2">
      <div className="stat-row span-2">
        <Stat label="Account holders" plain={String(accounts.length)} />
        <Stat label="Total on ledger" value={totals.ledger} />
        <Stat label="Available" value={totals.available} />
        <Stat label="Awaiting approval" value={totals.pending} />
      </div>

      <Panel
        className="span-2"
        title="Your general ledger"
        sub="How value flows through this household."
      >
        <p className="prose">
          Every {b.coin_name} your holders own was minted from your household's{" "}
          <b>Issuance GL</b> and will retire to your <b>Redemption GL</b> when
          spent. Wallets and the two GL accounts always net to zero — that's
          double-entry.{" "}
          <Info text="A general ledger (GL) holds the internal accounts a bank posts against. Each household here keeps its own, isolated from every other household." />
        </p>
      </Panel>

      <Panel className="span-2" title="Recent activity">
        <TransactionsPanel txs={recent.slice(0, 8)} accountNameFor={nameFor} />
      </Panel>
    </div>
  );
}

function Stat({
  label,
  value,
  plain,
}: {
  label: string;
  value?: number;
  plain?: string;
}) {
  return (
    <div className="stat-card">
      <div className="stat-label">{label}</div>
      <div className="stat-value">
        {plain !== undefined ? plain : <Money minor={value ?? 0} />}
      </div>
    </div>
  );
}

function Approvals({
  nameFor,
  onChange,
}: {
  nameFor: (id: string) => string;
  onChange: () => void;
}) {
  const [pending, setPending] = useState<Transaction[]>([]);
  const { err, run } = useError();

  const load = useCallback(async () => {
    const { transactions } = await api.pendingTransactions();
    setPending(transactions ?? []);
  }, []);
  useEffect(() => {
    load().catch(() => {});
  }, [load]);

  const decide = (fn: () => Promise<unknown>) =>
    run(fn, () => {
      load();
      onChange();
    });

  return (
    <Panel
      title="Pending approvals"
      sub="Each item is an authorization hold — approve to settle it, decline to void it, or adjust the reward first."
    >
      <Notice err={err} />
      {pending.length === 0 ? (
        <Empty
          title="Queue is clear."
          hint="New chore and reward requests will land here."
        />
      ) : (
        <ul className="rows">
          {pending.map((t) => (
            <ApprovalRow
              key={t.id}
              t={t}
              nameFor={nameFor}
              onSettle={() => decide(() => api.settle(t.id))}
              onVoid={() => decide(() => api.void(t.id))}
              onAdjust={(amount) => decide(() => api.adjustReward(t.id, amount))}
            />
          ))}
        </ul>
      )}
    </Panel>
  );
}

function ApprovalRow({
  t,
  nameFor,
  onSettle,
  onVoid,
  onAdjust,
}: {
  t: Transaction;
  nameFor: (id: string) => string;
  onSettle: () => void;
  onVoid: () => void;
  onAdjust: (amount: string) => void;
}) {
  const isCustomChore = t.type === "earn" && !t.task_id;
  const [adjusting, setAdjusting] = useState(false);
  const [amount, setAmount] = useState(coins(t.amount_minor));

  return (
    <li className="row">
      <div>
        <div className="row-title">
          {nameFor(t.account_id)}{" "}
          {isCustomChore && <span className="chip chip-bounty">proposed chore</span>}
        </div>
        <div className="row-sub">
          {t.type === "earn" ? "Chore" : "Reward"} · {t.memo} ·{" "}
          {new Date(t.created_at).toLocaleDateString()}
        </div>
        {adjusting && (
          <div className="hint-line adjust-row">
            <input
              className="adjust-input"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="0.25"
            />
            <button
              className="btn-primary sm"
              onClick={() => {
                onAdjust(amount);
                setAdjusting(false);
              }}
            >
              Save
            </button>
            <button className="btn-ghost sm" onClick={() => setAdjusting(false)}>
              Cancel
            </button>
          </div>
        )}
      </div>
      <div className="row-right">
        <Money
          minor={t.amount_minor}
          signed
          className={isCredit(t) ? "pos" : "neg"}
        />
        {t.type === "earn" && !adjusting && (
          <button className="btn-ghost sm" onClick={() => setAdjusting(true)}>
            <IconAdjust width={15} height={15} /> Adjust
          </button>
        )}
        <button className="btn-primary sm" onClick={onSettle}>
          <IconCheck width={15} height={15} /> Approve
        </button>
        <button className="btn-danger sm" onClick={onVoid}>
          <IconX width={15} height={15} /> Decline
        </button>
      </div>
    </li>
  );
}

function Accounts({ onChange }: { onChange: () => void }) {
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selected, setSelected] = useState<Account | null>(null);
  const { err, run } = useError();
  const [form, setForm] = useState({
    type: "holder",
    display_name: "",
    username: "",
    password: "",
  });
  const [msg, setMsg] = useState("");

  const load = useCallback(async () => {
    const [{ customers }, { accounts }] = await Promise.all([
      api.listCustomers(),
      api.listAccounts(),
    ]);
    setCustomers(customers ?? []);
    setAccounts(accounts ?? []);
  }, []);
  useEffect(() => {
    load().catch(() => {});
  }, [load]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setMsg("");
    run(
      () =>
        api.createCustomer(
          form.type,
          form.display_name,
          form.username,
          form.password,
        ),
      () => {
        setMsg(`Opened account for ${form.display_name || form.username}`);
        setForm({
          type: "holder",
          display_name: "",
          username: "",
          password: "",
        });
        load();
        onChange();
      },
    );
  };

  if (selected)
    return (
      <AccountDetail
        account={selected}
        onBack={() => {
          setSelected(null);
          load();
        }}
      />
    );

  return (
    <div className="grid-2">
      <Panel
        title="Account holders"
        sub="Select a holder to open their statement."
      >
        {accounts.length === 0 ? (
          <Empty title="No accounts yet." hint="Open one on the right." />
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Holder</th>
                <th>Account No.</th>
                <th className="amt">Available</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {accounts.map((a) => {
                const cust = customers.find((c) => c.id === a.customer_id);
                return (
                  <tr
                    key={a.id}
                    className="ledger-row"
                    onClick={() => setSelected(a)}
                    tabIndex={0}
                    onKeyDown={(e) => e.key === "Enter" && setSelected(a)}
                  >
                    <td>{cust?.display_name ?? a.name}</td>
                    <td className="mono muted">
                      {a.id.slice(0, 8).toUpperCase()}
                    </td>
                    <td className="amt">
                      <Money minor={a.balance?.available_minor ?? 0} />
                    </td>
                    <td className="go">
                      <IconArrowRight width={16} height={16} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel
        title="Open an account"
        sub="Onboard a kid (holder) or a co-parent (operator)."
      >
        <form onSubmit={submit} className="form">
          <label className="field">
            Type
            <select
              value={form.type}
              onChange={(e) => setForm({ ...form, type: e.target.value })}
            >
              <option value="holder">Holder (kid)</option>
              <option value="operator">Operator (parent)</option>
            </select>
          </label>
          <label className="field">
            Full name
            <input
              value={form.display_name}
              onChange={(e) =>
                setForm({ ...form, display_name: e.target.value })
              }
            />
          </label>
          <label className="field">
            Username
            <input
              value={form.username}
              onChange={(e) => setForm({ ...form, username: e.target.value })}
              autoComplete="off"
            />
          </label>
          <label className="field">
            Password
            <input
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              autoComplete="new-password"
            />
          </label>
          <Notice msg={msg} err={err} />
          <button className="btn-primary">Open account</button>
        </form>
      </Panel>
    </div>
  );
}

function AccountDetail({
  account,
  onBack,
}: {
  account: Account;
  onBack: () => void;
}) {
  const [txs, setTxs] = useState<Transaction[]>([]);
  useEffect(() => {
    api
      .accountTransactions(account.id)
      .then(({ transactions }) => setTxs(transactions ?? []))
      .catch(() => {});
  }, [account.id]);
  return (
    <div className="grid-2">
      <div className="span-2 detail-head">
        <button className="btn-ghost" onClick={onBack}>
          ← All accounts
        </button>
      </div>
      <Panel
        className="span-2"
        title={account.name}
        sub={`Account No. ${account.id.slice(0, 8).toUpperCase()}`}
      >
        <BalanceTiles balance={account.balance} />
      </Panel>
      <Panel
        className="span-2"
        title="Statement"
        sub="Select a posting to inspect its double-entry detail."
      >
        <TransactionsPanel txs={txs} accountNameFor={() => account.name} />
      </Panel>
      <div className="span-2">
        <Inbox accountId={account.id} />
      </div>
      <div className="span-2">
        <AccountCharts accountId={account.id} />
      </div>
    </div>
  );
}

function Activity({ nameFor }: { nameFor: (id: string) => string }) {
  const [txs, setTxs] = useState<Transaction[]>([]);
  const [filter, setFilter] = useState("");
  const load = useCallback(async () => {
    const { transactions } = await api.listTransactions(filter);
    setTxs(transactions ?? []);
  }, [filter]);
  useEffect(() => {
    load().catch(() => {});
  }, [load]);

  const filters = [
    ["", "All"],
    ["pending", "Pending"],
    ["settled", "Settled"],
    ["voided", "Voided"],
  ];
  return (
    <Panel
      title="Activity"
      sub="Every posting across the household. Select a row for the ledger detail."
      actions={
        <div className="segmented">
          {filters.map(([v, l]) => (
            <button
              key={v}
              className={filter === v ? "seg is-active" : "seg"}
              onClick={() => setFilter(v)}
            >
              {l}
            </button>
          ))}
        </div>
      }
    >
      <TransactionsPanel txs={txs} accountNameFor={nameFor} />
    </Panel>
  );
}

function Adjust({ onChange }: { onChange: () => void }) {
  const b = useBranding();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [txs, setTxs] = useState<Transaction[]>([]);
  const { err, run } = useError();
  const [form, setForm] = useState({
    accountId: "",
    direction: "credit",
    amount: "",
    reason: "",
    occurred_at: "",
    note: "",
  });
  const [msg, setMsg] = useState("");

  const load = useCallback(async () => {
    const { accounts } = await api.listAccounts();
    setAccounts(accounts ?? []);
    setForm((f) => ({ ...f, accountId: f.accountId || accounts[0]?.id || "" }));
  }, []);
  useEffect(() => {
    load().catch(() => {});
  }, [load]);

  const loadTxs = useCallback(async (id: string) => {
    if (!id) return;
    const { transactions } = await api.accountTransactions(id);
    setTxs(transactions ?? []);
  }, []);
  useEffect(() => {
    loadTxs(form.accountId).catch(() => {});
  }, [form.accountId, loadTxs]);

  const selected = accounts.find((a) => a.id === form.accountId);
  const nameFor = () => selected?.name ?? "Wallet";

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setMsg("");
    run(
      () =>
        api.adjust(form.accountId, {
          direction: form.direction as "credit" | "debit",
          amount: form.amount,
          reason: form.reason,
          occurred_at: form.occurred_at || undefined,
          details: form.note ? { note: form.note } : undefined,
        }),
      async () => {
        setMsg(
          `${form.direction === "credit" ? "Credited" : "Debited"} ${form.amount} ${b.coin_code}`,
        );
        setForm((f) => ({ ...f, amount: "", reason: "", note: "" }));
        await load();
        await loadTxs(form.accountId);
        onChange();
      },
    );
  };

  return (
    <div className="grid-2">
      <Panel
        title="Manual journal entry"
        sub="Post directly to an account. Settles immediately — no approval hold."
      >
        <form onSubmit={submit} className="form">
          <label className="field">
            Account
            <select
              value={form.accountId}
              onChange={(e) => setForm({ ...form, accountId: e.target.value })}
            >
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            Direction
            <select
              value={form.direction}
              onChange={(e) => setForm({ ...form, direction: e.target.value })}
            >
              <option value="credit">Credit — add {b.coin_name_plural}</option>
              <option value="debit">
                Debit — subtract {b.coin_name_plural}
              </option>
            </select>
          </label>
          <label className="field">
            Amount ({b.coin_name_plural})
            <input
              value={form.amount}
              onChange={(e) => setForm({ ...form, amount: e.target.value })}
              placeholder="0.50"
            />
          </label>
          <label className="field">
            Reason
            <input
              value={form.reason}
              onChange={(e) => setForm({ ...form, reason: e.target.value })}
              placeholder="Birthday bonus"
            />
          </label>
          <label className="field">
            Value date <span className="field-opt">optional</span>
            <input
              type="date"
              value={form.occurred_at}
              onChange={(e) =>
                setForm({ ...form, occurred_at: e.target.value })
              }
            />
          </label>
          <label className="field">
            Note <span className="field-opt">optional</span>
            <input
              value={form.note}
              onChange={(e) => setForm({ ...form, note: e.target.value })}
            />
          </label>
          {selected?.balance && (
            <div className="hint-line">
              Available now: <Money minor={selected.balance.available_minor} />
            </div>
          )}
          <Notice msg={msg} err={err} />
          <button className="btn-primary">Post entry</button>
        </form>
      </Panel>
      <Panel title="Recent activity" sub="On the selected account.">
        <TransactionsPanel txs={txs} accountNameFor={nameFor} />
      </Panel>
    </div>
  );
}

function Chores() {
  const b = useBranding();
  const [tasks, setTasks] = useState<Task[]>([]);
  const { err, run } = useError();
  const [form, setForm] = useState({ name: "", description: "", value: "" });
  const [bountyErr, setBountyErr] = useState("");
  const [bountyMsg, setBountyMsg] = useState("");
  const [bountyForm, setBountyForm] = useState({ name: "", description: "", value: "", expiresAt: "" });

  const load = useCallback(async () => {
    const { tasks } = await api.listTasks();
    setTasks(tasks ?? []);
  }, []);
  useEffect(() => {
    load().catch(() => {});
  }, [load]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(
      () => api.createTask(form.name, form.description, form.value),
      () => {
        setForm({ name: "", description: "", value: "" });
        load();
      },
    );
  };

  const submitBounty = async (e: FormEvent) => {
    e.preventDefault();
    setBountyErr("");
    setBountyMsg("");
    try {
      await api.createTask(bountyForm.name, bountyForm.description, bountyForm.value, {
        isBounty: true,
        expiresAt: bountyForm.expiresAt ? new Date(bountyForm.expiresAt).toISOString() : undefined,
      });
      setBountyMsg(
        bountyForm.expiresAt
          ? `Posted "${bountyForm.name}" — it expires ${new Date(bountyForm.expiresAt).toLocaleString()}.`
          : `Posted "${bountyForm.name}" — every kid will see it until one claims it.`,
      );
      setBountyForm({ name: "", description: "", value: "", expiresAt: "" });
      load();
    } catch (e) {
      setBountyErr(e instanceof ApiError ? e.message : String(e));
    }
  };

  // Bounties sink to the bottom so the regular catalog reads cleanly; order
  // within each group is otherwise unchanged (stable sort).
  const sortedTasks = useMemo(
    () => [...tasks].sort((a, b) => Number(a.is_bounty) - Number(b.is_bounty)),
    [tasks],
  );

  return (
    <div className="grid-2">
      <Panel
        className="span-2"
        title="Chore catalog"
        sub="What holders can earn. Retire a chore to hide it without deleting history."
      >
        {tasks.length === 0 ? (
          <Empty title="No chores yet." hint="Add one below." />
        ) : (
          <ul className="rows">
            {sortedTasks.map((t) => {
              const expired = !!t.expires_at && new Date(t.expires_at) <= new Date();
              const bountyStatus = t.claimed_by ? "claimed" : expired ? "expired" : "open";
              return (
              <li key={t.id} className="row">
                <div>
                  <div className="row-title">
                    {t.name}{" "}
                    {t.is_bounty && (
                      <span className="chip chip-bounty">bounty · {bountyStatus}</span>
                    )}
                    {!t.active && (
                      <span className="chip chip-off">retired</span>
                    )}
                  </div>
                  {t.description && (
                    <div className="row-sub">{t.description}</div>
                  )}
                  {t.is_bounty && t.expires_at && (
                    <div className="row-sub">
                      {expired ? "Expired" : "Expires"} {new Date(t.expires_at).toLocaleString()}
                    </div>
                  )}
                </div>
                <div className="row-right">
                  <Money minor={t.value_minor} signed className="pos" />
                  <button
                    className="btn-ghost sm"
                    onClick={() =>
                      run(
                        () => api.updateTask(t.id, { active: !t.active }),
                        load,
                      )
                    }
                  >
                    {t.active ? "Retire" : "Restore"}
                  </button>
                </div>
              </li>
              );
            })}
          </ul>
        )}
      </Panel>
      <Panel
        title="Add a chore"
        sub={`Set what it's worth in ${b.coin_name_plural}.`}
      >
        <form onSubmit={submit} className="form">
          <label className="field">
            Name
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="field">
            Description
            <input
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </label>
          <label className="field">
            Value ({b.coin_name_plural}, e.g. 0.15)
            <input
              value={form.value}
              onChange={(e) => setForm({ ...form, value: e.target.value })}
            />
          </label>
          <Notice err={err} />
          <button className="btn-primary">Add chore</button>
        </form>
      </Panel>
      <Panel
        title="Post a bounty"
        sub="A one-time opportunity — every kid sees it, first to claim it wins."
      >
        <form onSubmit={submitBounty} className="form">
          <label className="field">
            Name
            <input
              value={bountyForm.name}
              onChange={(e) => setBountyForm({ ...bountyForm, name: e.target.value })}
            />
          </label>
          <label className="field">
            Description
            <input
              value={bountyForm.description}
              onChange={(e) =>
                setBountyForm({ ...bountyForm, description: e.target.value })
              }
            />
          </label>
          <label className="field">
            Value ({b.coin_name_plural}, e.g. 1.00)
            <input
              value={bountyForm.value}
              onChange={(e) => setBountyForm({ ...bountyForm, value: e.target.value })}
            />
          </label>
          <label className="field">
            Expires <span className="field-opt">optional — leave blank for no deadline</span>
            <input
              type="datetime-local"
              value={bountyForm.expiresAt}
              onChange={(e) => setBountyForm({ ...bountyForm, expiresAt: e.target.value })}
            />
          </label>
          <Notice msg={bountyMsg} err={bountyErr} />
          <button className="btn-primary">
            <IconZap width={16} height={16} /> Post bounty
          </button>
        </form>
      </Panel>
    </div>
  );
}

function Audit() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  useEffect(() => {
    api
      .audit()
      .then(({ events }) => setEvents(events ?? []))
      .catch(() => {});
  }, []);
  return (
    <Panel
      title="Audit trail"
      sub="An append-only log — records are only ever added, never edited or removed."
    >
      {events.length === 0 ? (
        <Empty title="No events yet." />
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Action</th>
              <th>Entity</th>
              <th>When</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={e.id}>
                <td>
                  <span className="chip">{e.action}</span>
                </td>
                <td className="mono muted">
                  {e.entity_type}/{e.entity_id.slice(0, 8)}
                </td>
                <td className="muted nowrap">
                  {new Date(e.created_at).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Panel>
  );
}
