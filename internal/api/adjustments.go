package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/money"

	"github.com/google/uuid"
)

type adjustmentRequest struct {
	Direction  string         `json:"direction"`   // "credit" (add) or "debit" (subtract)
	Amount     string         `json:"amount"`      // coin string, e.g. "0.50"
	Reason     string         `json:"reason"`      // why the adjustment was made
	OccurredAt string         `json:"occurred_at"` // optional: "YYYY-MM-DD" or RFC3339
	Details    map[string]any `json:"details"`     // optional free-form detail
}

// handleCreateAdjustment lets an operator directly add or subtract coins from an
// account (a manual journal entry). It posts immediately — no approval hold —
// and records descriptive metadata (reason, effective date, free-form details).
func (s *Server) handleCreateAdjustment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	if acct.Kind != domain.AccountCustomer {
		writeErr(w, http.StatusBadRequest, "bad_request", "can only adjust customer accounts")
		return
	}
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	var req adjustmentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	amount, err := money.ParseCoins(req.Amount)
	if err != nil || amount <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "amount must be a positive coin value, e.g. \"0.50\"")
		return
	}
	occurredAt, err := parseOccurred(req.OccurredAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "occurred_at must be a date (YYYY-MM-DD) or RFC3339 timestamp")
		return
	}

	var (
		transferID string
		txType     domain.TxType
		glID       string
	)
	switch req.Direction {
	case "credit":
		transferID, err = s.ledger.Credit(tenant.IssuanceTBID, acct.TBAccountID, amount)
		txType, glID = domain.TxAdjustCredit, tenant.IssuanceAccountID
	case "debit":
		transferID, err = s.ledger.Debit(acct.TBAccountID, tenant.RedemptionTBID, amount)
		txType, glID = domain.TxAdjustDebit, tenant.RedemptionAccountID
	default:
		writeErr(w, http.StatusBadRequest, "bad_request", "direction must be 'credit' or 'debit'")
		return
	}
	if errors.Is(err, ledger.ErrInsufficientFunds) {
		writeErr(w, http.StatusConflict, "insufficient_funds", "account does not have enough available coins for this debit")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to post adjustment: "+err.Error())
		return
	}

	memo := strings.TrimSpace(req.Reason)
	if memo == "" {
		memo = "Manual adjustment"
	}
	now := nowUTC()
	aid := actor.IdentityID
	tx := &domain.Transaction{
		ID: uuid.NewString(), TenantID: actor.TenantID, Type: txType, Status: domain.TxSettled,
		AccountID: acct.ID, GLAccountID: glID, AmountMinor: amount,
		Memo: memo, TBPendingTransferID: transferID, EffectiveAt: occurredAt, Details: req.Details,
		CreatedBy: actor.IdentityID, DecidedBy: &aid, DecidedAt: &now,
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record adjustment")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "adjustment.create", "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "direction": req.Direction, "amount_minor": amount, "reason": memo})
	writeJSON(w, http.StatusCreated, tx)
}

// parseOccurred accepts a date-only or RFC3339 string; empty means "not set".
func parseOccurred(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if tm, err := time.Parse(layout, s); err == nil {
			u := tm.UTC()
			return &u, nil
		}
	}
	return nil, errors.New("invalid date")
}
