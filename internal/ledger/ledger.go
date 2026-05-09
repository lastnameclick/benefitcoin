// Package ledger is the TigerBeetle adapter. Earnings credit a customer account
// from its household's Issuance GL; redemptions debit it to the Redemption GL.
// Requests run as two-phase transfers: a pending hold, later posted (settle) or
// voided.
package ledger

import (
	"errors"
	"fmt"
	"math/big"

	"cpal/internal/domain"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

const (
	ledgerBNC uint32 = 1 // single ledger for the BNC currency

	codeEarn         uint16 = 1
	codeRedeem       uint16 = 2
	codeAdjustCredit uint16 = 3   // operator manual credit
	codeAdjustDebit  uint16 = 4   // operator manual debit
	codeGL           uint16 = 900 // internal general-ledger account code
	codeWallet       uint16 = 1   // customer wallet account code
)

// ErrInsufficientFunds means a redemption hold would overdraw the account.
var ErrInsufficientFunds = errors.New("insufficient funds")

// Ledger wraps a TigerBeetle client.
type Ledger struct {
	client tb.Client
}

// Connect opens a TigerBeetle client connection.
func Connect(clusterID uint64, addresses []string) (*Ledger, error) {
	c, err := tb.NewClient(tb.ToUint128(clusterID), addresses)
	if err != nil {
		return nil, fmt.Errorf("connect tigerbeetle: %w", err)
	}
	return &Ledger{client: c}, nil
}

func (l *Ledger) Close() { l.client.Close() }

// OpenGL creates a new internal general-ledger account and returns its
// TigerBeetle id. GL accounts have no overdraft constraint — the Issuance GL is
// meant to run a debit balance as it mints coins. A household opens two of
// these (issuance + redemption) at signup.
func (l *Ledger) OpenGL() (string, error) {
	id := tb.ID()
	acct := tb.Account{
		ID:     id,
		Ledger: ledgerBNC,
		Code:   codeGL,
		Flags:  tb.AccountFlags{History: true}.ToUint16(),
	}
	res, err := l.client.CreateAccounts([]tb.Account{acct})
	if err != nil {
		return "", err
	}
	if err := checkAccountResults(res); err != nil {
		return "", err
	}
	return uint128ToStr(id), nil
}

// OpenAccount creates a new customer wallet account that may not be overdrawn,
// returning its TigerBeetle id as a decimal string.
func (l *Ledger) OpenAccount() (string, error) {
	id := tb.ID()
	acct := tb.Account{
		ID:     id,
		Ledger: ledgerBNC,
		Code:   codeWallet,
		Flags:  tb.AccountFlags{DebitsMustNotExceedCredits: true, History: true}.ToUint16(),
	}
	res, err := l.client.CreateAccounts([]tb.Account{acct})
	if err != nil {
		return "", err
	}
	if err := checkAccountResults(res); err != nil {
		return "", err
	}
	return uint128ToStr(id), nil
}

// EarnHold places a hold that will mint `amount` minor units from the given
// Issuance GL into the customer account once settled. Returns the pending
// transfer id.
func (l *Ledger) EarnHold(issuanceTBID, customerTBID string, amount int64) (string, error) {
	return l.hold(issuanceTBID, customerTBID, amount, codeEarn)
}

// RedeemHold places a hold that will move `amount` minor units from the customer
// account to the given Redemption GL once settled. Returns ErrInsufficientFunds
// if the account cannot cover it (enforced by TigerBeetle at hold-creation time).
func (l *Ledger) RedeemHold(customerTBID, redemptionTBID string, amount int64) (string, error) {
	return l.hold(customerTBID, redemptionTBID, amount, codeRedeem)
}

func (l *Ledger) hold(debitTBID, creditTBID string, amount int64, code uint16) (string, error) {
	debit, err := strToUint128(debitTBID)
	if err != nil {
		return "", err
	}
	credit, err := strToUint128(creditTBID)
	if err != nil {
		return "", err
	}
	id := tb.ID()
	t := tb.Transfer{
		ID:              id,
		DebitAccountID:  debit,
		CreditAccountID: credit,
		Amount:          tb.ToUint128(uint64(amount)),
		Ledger:          ledgerBNC,
		Code:            code,
		Flags:           tb.TransferFlags{Pending: true}.ToUint16(),
	}
	res, err := l.client.CreateTransfers([]tb.Transfer{t})
	if err != nil {
		return "", err
	}
	if err := checkTransferResults(res); err != nil {
		return "", err
	}
	return uint128ToStr(id), nil
}

// Credit immediately posts `amount` minor units from the given Issuance GL into
// the customer account (an operator manual credit). Returns the transfer id.
func (l *Ledger) Credit(issuanceTBID, customerTBID string, amount int64) (string, error) {
	return l.post(issuanceTBID, customerTBID, amount, codeAdjustCredit)
}

// Debit immediately posts `amount` minor units from the customer account to the
// given Redemption GL (an operator manual debit). Returns ErrInsufficientFunds
// if the account cannot cover it.
func (l *Ledger) Debit(customerTBID, redemptionTBID string, amount int64) (string, error) {
	return l.post(customerTBID, redemptionTBID, amount, codeAdjustDebit)
}

// post creates a single-phase (immediately settled) transfer.
func (l *Ledger) post(debitTBID, creditTBID string, amount int64, code uint16) (string, error) {
	debit, err := strToUint128(debitTBID)
	if err != nil {
		return "", err
	}
	credit, err := strToUint128(creditTBID)
	if err != nil {
		return "", err
	}
	id := tb.ID()
	t := tb.Transfer{
		ID:              id,
		DebitAccountID:  debit,
		CreditAccountID: credit,
		Amount:          tb.ToUint128(uint64(amount)),
		Ledger:          ledgerBNC,
		Code:            code,
	}
	res, err := l.client.CreateTransfers([]tb.Transfer{t})
	if err != nil {
		return "", err
	}
	if err := checkTransferResults(res); err != nil {
		return "", err
	}
	return uint128ToStr(id), nil
}

// Settle posts a pending transfer in full (approval). Returns the post transfer id.
func (l *Ledger) Settle(pendingID string, txType domain.TxType) (string, error) {
	return l.resolve(pendingID, txType, true)
}

// Void cancels a pending transfer (rejection). Returns the void transfer id.
func (l *Ledger) Void(pendingID string, txType domain.TxType) (string, error) {
	return l.resolve(pendingID, txType, false)
}

func (l *Ledger) resolve(pendingID string, txType domain.TxType, post bool) (string, error) {
	pID, err := strToUint128(pendingID)
	if err != nil {
		return "", err
	}
	id := tb.ID()
	t := tb.Transfer{
		ID:        id,
		PendingID: pID,
		Ledger:    ledgerBNC,
		Code:      codeFor(txType),
	}
	if post {
		t.Amount = tb.AmountMax // post the full pending amount
		t.Flags = tb.TransferFlags{PostPendingTransfer: true}.ToUint16()
	} else {
		t.Flags = tb.TransferFlags{VoidPendingTransfer: true}.ToUint16()
	}
	res, err := l.client.CreateTransfers([]tb.Transfer{t})
	if err != nil {
		return "", err
	}
	if err := checkTransferResults(res); err != nil {
		return "", err
	}
	return uint128ToStr(id), nil
}

// Balance returns the live balance fields for a customer account.
func (l *Ledger) Balance(customerTBID string) (domain.Balance, error) {
	id, err := strToUint128(customerTBID)
	if err != nil {
		return domain.Balance{}, err
	}
	accts, err := l.client.LookupAccounts([]tb.Uint128{id})
	if err != nil {
		return domain.Balance{}, err
	}
	if len(accts) == 0 {
		return domain.Balance{}, fmt.Errorf("account %s not found in ledger", customerTBID)
	}
	a := accts[0]
	return domain.Balance{
		CreditsPosted:  u128ToInt64(a.CreditsPosted),
		DebitsPosted:   u128ToInt64(a.DebitsPosted),
		CreditsPending: u128ToInt64(a.CreditsPending),
		DebitsPending:  u128ToInt64(a.DebitsPending),
	}, nil
}

func codeFor(txType domain.TxType) uint16 {
	if txType == domain.TxRedeem {
		return codeRedeem
	}
	return codeEarn
}

func checkAccountResults(res []tb.CreateAccountResult) error {
	for _, r := range res {
		switch r.Status {
		case tb.AccountCreated, tb.AccountExists:
			// ok / idempotent
		default:
			return fmt.Errorf("create account failed: %s", r.Status)
		}
	}
	return nil
}

func checkTransferResults(res []tb.CreateTransferResult) error {
	for _, r := range res {
		switch r.Status {
		case tb.TransferCreated:
			// ok
		case tb.TransferExceedsCredits:
			return ErrInsufficientFunds
		default:
			return fmt.Errorf("create transfer failed: %s", r.Status)
		}
	}
	return nil
}

func strToUint128(s string) (tb.Uint128, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return tb.Uint128{}, fmt.Errorf("invalid ledger id %q", s)
	}
	return tb.BigIntToUint128(n), nil
}

func uint128ToStr(v tb.Uint128) string { return v.BigInt().String() }

func u128ToInt64(v tb.Uint128) int64 { return v.BigInt().Int64() }
