import { isCredit, type Transaction } from "../api";

// A single leg of a double-entry posting.
export interface Leg {
  side: "debit" | "credit";
  account: string;
}

// postingLegs derives the two ledger legs a transaction moves. Every movement
// debits one account and credits another for the same amount:
//   earn / adjust_credit:  DR Issuance GL   → CR wallet   (money into the wallet)
//   redeem / adjust_debit: DR wallet        → CR Redemption GL
export function postingLegs(t: Transaction, walletName: string): Leg[] {
  const wallet = walletName || "Wallet";
  if (isCredit(t)) {
    return [
      { side: "debit", account: "Issuance GL" },
      { side: "credit", account: wallet },
    ];
  }
  return [
    { side: "debit", account: wallet },
    { side: "credit", account: "Redemption GL" },
  ];
}

export type StepState = "done" | "current" | "todo" | "bad";
export interface Step {
  label: string;
  state: StepState;
  hint: string;
}

// lifecycle describes where a transaction sits in the authorization → settlement
// flow, so the UI can teach the difference between a hold and a posted entry.
export function lifecycle(t: Transaction): Step[] {
  const single = t.type === "adjust_credit" || t.type === "adjust_debit";
  if (single) {
    return [
      { label: "Requested", state: "done", hint: "Operator initiated a manual journal entry." },
      { label: "Posted", state: "done", hint: "Single-phase transfer — settled immediately, no hold." },
    ];
  }
  if (t.status === "pending") {
    return [
      { label: "Requested", state: "done", hint: "Holder submitted the request." },
      { label: "Authorization hold", state: "current", hint: "Funds are reserved as a pending transfer, not yet spendable." },
      { label: "Settlement", state: "todo", hint: "Awaiting an operator to post (approve) or void (decline)." },
    ];
  }
  if (t.status === "settled") {
    return [
      { label: "Requested", state: "done", hint: "Holder submitted the request." },
      { label: "Hold placed", state: "done", hint: "Funds were reserved on the ledger." },
      { label: "Settled", state: "done", hint: "The pending transfer was posted — the balance moved." },
    ];
  }
  return [
    { label: "Requested", state: "done", hint: "Holder submitted the request." },
    { label: "Hold placed", state: "done", hint: "Funds were reserved on the ledger." },
    { label: "Voided", state: "bad", hint: "The hold was reversed — no balance change." },
  ];
}

// A one-line, plain-language explanation of what a transaction did.
export function txExplainer(t: Transaction): string {
  switch (t.type) {
    case "earn":
      return "New value is minted from the Issuance GL and credited to the wallet once settled.";
    case "redeem":
      return "Value is debited from the wallet and retired to the Redemption GL when settled.";
    case "adjust_credit":
      return "A manual credit posted directly from the Issuance GL into the wallet.";
    case "adjust_debit":
      return "A manual debit posted directly from the wallet to the Redemption GL.";
  }
}
