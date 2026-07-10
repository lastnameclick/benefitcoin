package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type earningRequest struct {
	TaskID string `json:"task_id"`
}

// handleCreateEarning lets a holder report a completed task, placing an
// authorization hold that mints the task's value once an operator settles it.
// If the task is a one-time bounty, this also claims it — the first holder to
// submit wins, and it disappears from the catalog for everyone else.
func (s *Server) handleCreateEarning(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	var req earningRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	task, err := s.store.GetTask(r.Context(), actor.TenantID, req.TaskID)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "task not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load task")
		return
	}
	if !task.Active {
		writeErr(w, http.StatusBadRequest, "task_inactive", "that task is no longer available")
		return
	}
	if task.IsBounty {
		err := s.store.ClaimBounty(r.Context(), actor.TenantID, task.ID, actor.CustomerID)
		switch {
		case err == store.ErrExpired:
			writeErr(w, http.StatusConflict, "bounty_expired", "this bounty's deadline has passed")
			return
		case err == store.ErrConflict:
			writeErr(w, http.StatusConflict, "bounty_claimed", "someone already claimed this bounty")
			return
		case err != nil:
			writeErr(w, http.StatusInternalServerError, "internal", "failed to claim bounty")
			return
		}
	}

	pendingID, err := s.ledger.EarnHold(tenant.IssuanceTBID, acct.TBAccountID, task.ValueMinor)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to place earn hold: "+err.Error())
		return
	}
	taskID := task.ID
	tx := &domain.Transaction{
		ID: uuid.NewString(), TenantID: actor.TenantID, Type: domain.TxEarn, Status: domain.TxPending,
		AccountID: acct.ID, GLAccountID: tenant.IssuanceAccountID, AmountMinor: task.ValueMinor,
		TaskID: &taskID, Memo: task.Name, TBPendingTransferID: pendingID, CreatedBy: actor.IdentityID,
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record transaction")
		return
	}
	action := "earning.request"
	if task.IsBounty {
		action = "bounty.claim"
	}
	s.audit(r.Context(), actor.IdentityID, action, "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "task_id": task.ID, "amount_minor": task.ValueMinor})
	if task.IsBounty {
		s.notifier.NotifyOperators(r.Context(), actor.TenantID, domain.NotifyBountyClaimed,
			"Bounty claimed", task.Name+" was claimed and is awaiting your review.",
			map[string]any{"transaction_id": tx.ID, "task_id": task.ID})
	} else {
		s.notifier.NotifyOperators(r.Context(), actor.TenantID, domain.NotifyChoreSubmitted,
			task.Name+" submitted", "A chore was submitted for verification.",
			map[string]any{"transaction_id": tx.ID, "task_id": task.ID})
	}
	writeJSON(w, http.StatusCreated, tx)
}

type proposeChoreRequest struct {
	Description string `json:"description"`
	Value       string `json:"value"` // proposed coin amount, e.g. "0.25"
}

// handleProposeChore lets a holder submit a chore that isn't on the catalog —
// a free-text description and a proposed reward — as an authorization hold
// awaiting an operator's decision, same as a catalog earning.
func (s *Server) handleProposeChore(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	var req proposeChoreRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "description is required")
		return
	}
	valueMinor, err := money.ParseCoins(req.Value)
	if err != nil || valueMinor <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "value must be a positive coin amount, e.g. \"0.25\"")
		return
	}

	pendingID, err := s.ledger.EarnHold(tenant.IssuanceTBID, acct.TBAccountID, valueMinor)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to place earn hold: "+err.Error())
		return
	}
	tx := &domain.Transaction{
		ID: uuid.NewString(), TenantID: actor.TenantID, Type: domain.TxEarn, Status: domain.TxPending,
		AccountID: acct.ID, GLAccountID: tenant.IssuanceAccountID, AmountMinor: valueMinor,
		Memo: desc, TBPendingTransferID: pendingID, CreatedBy: actor.IdentityID,
		Details: map[string]any{"proposed_minor": valueMinor, "custom": true},
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record transaction")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "chore_proposal.create", "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "amount_minor": valueMinor, "description": desc})
	s.notifier.NotifyOperators(r.Context(), actor.TenantID, domain.NotifyChoreSubmitted,
		"Chore proposed", desc+" was proposed for verification.",
		map[string]any{"transaction_id": tx.ID})
	writeJSON(w, http.StatusCreated, tx)
}

type adjustTransactionRequest struct {
	Amount string `json:"amount"` // new coin amount, e.g. "0.30"
}

// handleAdjustTransaction lets an operator revise the reward on a pending earn
// request (typically a proposed chore) before approving it. The original hold
// is voided and replaced with a new one at the adjusted amount.
func (s *Server) handleAdjustTransaction(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	tx, err := s.store.GetTransaction(r.Context(), actor.TenantID, id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "transaction not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load transaction")
		return
	}
	if tx.Status != domain.TxPending {
		writeErr(w, http.StatusConflict, "already_decided", "transaction is already "+string(tx.Status))
		return
	}
	if tx.Type != domain.TxEarn {
		writeErr(w, http.StatusBadRequest, "bad_request", "only earn requests can be adjusted")
		return
	}
	var req adjustTransactionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	amountMinor, err := money.ParseCoins(req.Amount)
	if err != nil || amountMinor <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "amount must be a positive coin amount, e.g. \"0.30\"")
		return
	}

	acct, err := s.store.GetAccount(r.Context(), actor.TenantID, tx.AccountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load account")
		return
	}
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	if _, err := s.ledger.Void(tx.TBPendingTransferID, tx.Type); err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to release the prior hold: "+err.Error())
		return
	}
	newPendingID, err := s.ledger.EarnHold(tenant.IssuanceTBID, acct.TBAccountID, amountMinor)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to place adjusted hold: "+err.Error())
		return
	}
	if err := s.store.AdjustTransactionAmount(r.Context(), actor.TenantID, tx.ID, amountMinor, newPendingID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record adjustment")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "transaction.adjust", "transaction", tx.ID,
		map[string]any{"from_amount_minor": tx.AmountMinor, "to_amount_minor": amountMinor})

	updated, _ := s.store.GetTransaction(r.Context(), actor.TenantID, tx.ID)
	writeJSON(w, http.StatusOK, updated)
}

// handleCreateRedemption lets a holder request to spend one whole coin on a
// reward, or the discounted price if a flash sale is currently active. The
// hold reserves the funds; an operator settles to complete the spend.
func (s *Server) handleCreateRedemption(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	tenant, ok := s.loadTenant(w, r)
	if !ok {
		return
	}
	base := money.Coin(1) // generic 1-coin reward
	amount := base
	memo := "Reward redemption"
	var details map[string]any
	if sale, err := s.store.GetActiveFlashSale(r.Context(), actor.TenantID); err == nil {
		amount = sale.Apply(base)
		memo = "Reward redemption (flash sale)"
		details = map[string]any{"base_minor": base, "flash_sale_id": sale.ID, "discount_minor": base - amount}
	} else if err != store.ErrNotFound {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to check for a flash sale")
		return
	}

	pendingID, err := s.ledger.RedeemHold(acct.TBAccountID, tenant.RedemptionTBID, amount)
	if errors.Is(err, ledger.ErrInsufficientFunds) {
		writeErr(w, http.StatusConflict, "insufficient_funds", "not enough available coins to redeem a reward yet")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to place redeem hold: "+err.Error())
		return
	}
	tx := &domain.Transaction{
		ID: uuid.NewString(), TenantID: actor.TenantID, Type: domain.TxRedeem, Status: domain.TxPending,
		AccountID: acct.ID, GLAccountID: tenant.RedemptionAccountID, AmountMinor: amount,
		Memo: memo, TBPendingTransferID: pendingID, CreatedBy: actor.IdentityID, Details: details,
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record transaction")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "redemption.request", "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "amount_minor": amount})
	s.notifier.NotifyOperators(r.Context(), actor.TenantID, domain.NotifyRedemptionRequested,
		"Redemption requested", "A new redemption request is awaiting your review.",
		map[string]any{"transaction_id": tx.ID, "account_id": acct.ID})
	writeJSON(w, http.StatusCreated, tx)
}

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	status := r.URL.Query().Get("status")
	account := r.URL.Query().Get("account_id")
	txs, err := s.store.ListTransactions(r.Context(), actor.TenantID, status, account, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list transactions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": txs})
}

func (s *Server) handleSettle(w http.ResponseWriter, r *http.Request) { s.decide(w, r, true) }
func (s *Server) handleVoid(w http.ResponseWriter, r *http.Request)   { s.decide(w, r, false) }

// decide settles (post) or voids a pending authorization hold.
func (s *Server) decide(w http.ResponseWriter, r *http.Request, settle bool) {
	actor, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	tx, err := s.store.GetTransaction(r.Context(), actor.TenantID, id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "transaction not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load transaction")
		return
	}
	if tx.Status != domain.TxPending {
		writeErr(w, http.StatusConflict, "already_decided", "transaction is already "+string(tx.Status))
		return
	}

	var (
		postID  string
		action  string
		nextSts domain.TxStatus
	)
	if settle {
		postID, err = s.ledger.Settle(tx.TBPendingTransferID, tx.Type)
		action, nextSts = "transaction.settle", domain.TxSettled
	} else {
		postID, err = s.ledger.Void(tx.TBPendingTransferID, tx.Type)
		action, nextSts = "transaction.void", domain.TxVoided
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "ledger operation failed: "+err.Error())
		return
	}
	if err := s.store.DecideTransaction(r.Context(), actor.TenantID, tx.ID, nextSts, &postID, actor.IdentityID, nowUTC()); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record decision")
		return
	}
	// Declining a bounty claim reopens it for any holder to claim again.
	if !settle && tx.Type == domain.TxEarn && tx.TaskID != nil {
		if task, err := s.store.GetTask(r.Context(), actor.TenantID, *tx.TaskID); err == nil && task.IsBounty {
			_ = s.store.ReleaseBountyClaim(r.Context(), actor.TenantID, task.ID)
		}
	}
	s.audit(r.Context(), actor.IdentityID, action, "transaction", tx.ID,
		map[string]any{"amount_minor": tx.AmountMinor, "type": tx.Type})
	s.notifyDecision(r.Context(), tx, settle)

	updated, _ := s.store.GetTransaction(r.Context(), actor.TenantID, tx.ID)
	writeJSON(w, http.StatusOK, updated)
}

// notifyDecision alerts the holder whose transaction was just settled or
// voided — wording depends on whether it was a chore/bounty earning or a
// redemption. Best-effort: a failure to resolve the recipient never fails
// the caller's settle/void request.
func (s *Server) notifyDecision(ctx context.Context, tx domain.Transaction, settle bool) {
	acct, err := s.store.GetAccount(ctx, tx.TenantID, tx.AccountID)
	if err != nil || acct.CustomerID == nil {
		return
	}
	verb := "approved"
	if !settle {
		verb = "declined"
	}
	if tx.Type == domain.TxRedeem {
		s.notifier.NotifyCustomer(ctx, tx.TenantID, *acct.CustomerID, domain.NotifyRedemptionDecided,
			"Redemption "+verb, "Your redemption request was "+verb+".",
			map[string]any{"transaction_id": tx.ID})
		return
	}
	what := tx.Memo
	if what == "" {
		what = "Your chore"
	}
	s.notifier.NotifyCustomer(ctx, tx.TenantID, *acct.CustomerID, domain.NotifyChoreDecided,
		"Chore "+verb, what+" was "+verb+".",
		map[string]any{"transaction_id": tx.ID})
}
