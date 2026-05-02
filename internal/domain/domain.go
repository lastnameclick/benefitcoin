// Package domain holds the shared entities: tenants (households), customers
// (operators and holders), accounts, tasks, and transactions.
package domain

import "time"

// Tenant is a household — the isolation boundary for a SaaS deployment. Every
// customer, account, task, and transaction belongs to exactly one tenant, and
// each tenant keeps its own Issuance/Redemption GL accounts so its books balance
// independently.
type Tenant struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Status              string    `json:"status"`
	IssuanceAccountID   string    `json:"issuance_account_id"`   // Postgres GL account id
	RedemptionAccountID string    `json:"redemption_account_id"` // Postgres GL account id
	IssuanceTBID        string    `json:"issuance_tb_id"`        // TigerBeetle GL account id
	RedemptionTBID      string    `json:"redemption_tb_id"`      // TigerBeetle GL account id
	CreatedAt           time.Time `json:"created_at"`
}

type Role string

const (
	RoleOperator Role = "operator" // back-office / parent: can approve, administer
	RoleHolder   Role = "holder"   // account owner / kid: requests earnings & redemptions
)

func (r Role) Valid() bool { return r == RoleOperator || r == RoleHolder }

type CustomerType string

const (
	CustomerOperator CustomerType = "operator"
	CustomerHolder   CustomerType = "holder"
)

type AccountKind string

const (
	AccountCustomer AccountKind = "customer" // a holder's deposit account
	AccountInternal AccountKind = "internal" // an internal GL account (issuance/redemption)
)

type TxType string

const (
	TxEarn         TxType = "earn"          // mint coins for a completed task (GL -> account)
	TxRedeem       TxType = "redeem"        // spend coins on a reward (account -> GL)
	TxAdjustCredit TxType = "adjust_credit" // operator adds coins (GL -> account), posted immediately
	TxAdjustDebit  TxType = "adjust_debit"  // operator subtracts coins (account -> GL), posted immediately
)

type TxStatus string

const (
	TxPending TxStatus = "pending" // authorization hold placed, awaiting decision
	TxSettled TxStatus = "settled" // hold posted (approved)
	TxVoided  TxStatus = "voided"  // hold voided (rejected)
)

type Customer struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	Type        CustomerType `json:"type"`
	DisplayName string       `json:"display_name"`
	Status      string       `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
}

type Identity struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	CustomerID   string    `json:"customer_id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Account struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	CustomerID  *string     `json:"customer_id,omitempty"`
	Kind        AccountKind `json:"kind"`
	TBAccountID string      `json:"tb_account_id"`
	Currency    string      `json:"currency"`
	Product     string      `json:"product"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	OpenedAt    time.Time   `json:"opened_at"`
}

type Task struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ValueMinor  int64     `json:"value_minor"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

type Transaction struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	Type                TxType         `json:"type"`
	Status              TxStatus       `json:"status"`
	AccountID           string         `json:"account_id"`
	GLAccountID         string         `json:"gl_account_id"`
	AmountMinor         int64          `json:"amount_minor"`
	TaskID              *string        `json:"task_id,omitempty"`
	Memo                string         `json:"memo"`
	TBPendingTransferID string         `json:"tb_pending_transfer_id"`
	TBPostTransferID    *string        `json:"tb_post_transfer_id,omitempty"`
	EffectiveAt         *time.Time     `json:"effective_at,omitempty"`
	Details             map[string]any `json:"details,omitempty"`
	CreatedBy           string         `json:"created_by"`
	CreatedAt           time.Time      `json:"created_at"`
	DecidedBy           *string        `json:"decided_by,omitempty"`
	DecidedAt           *time.Time     `json:"decided_at,omitempty"`
}

type AuditEvent struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ActorIdentityID *string        `json:"actor_identity_id,omitempty"`
	Action          string         `json:"action"`
	EntityType      string         `json:"entity_type"`
	EntityID        string         `json:"entity_id"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Balance is the derived view of an account's funds, in minor units.
type Balance struct {
	CreditsPosted  int64 `json:"credits_posted"`
	DebitsPosted   int64 `json:"debits_posted"`
	CreditsPending int64 `json:"credits_pending"`
	DebitsPending  int64 `json:"debits_pending"`
}

// Current is the settled balance (credits_posted - debits_posted).
func (b Balance) Current() int64 { return b.CreditsPosted - b.DebitsPosted }

// Available is what can still be spent: settled balance minus outstanding
// redemption holds (debits_pending).
func (b Balance) Available() int64 { return b.Current() - b.DebitsPending }

// AwaitingApproval is the sum of pending earnings not yet settled.
func (b Balance) AwaitingApproval() int64 { return b.CreditsPending }
