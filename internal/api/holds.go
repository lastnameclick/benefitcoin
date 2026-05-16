package api

import (
	"errors"
	"net/http"

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
	s.audit(r.Context(), actor.IdentityID, "earning.request", "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "task_id": task.ID, "amount_minor": task.ValueMinor})
	writeJSON(w, http.StatusCreated, tx)
}

// handleCreateRedemption lets a holder request to spend one whole coin on a
// reward. The hold reserves the funds; an operator settles to complete the spend.
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
	amount := money.Coin(1) // generic 1-coin reward

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
		Memo: "Reward redemption", TBPendingTransferID: pendingID, CreatedBy: actor.IdentityID,
	}
	if err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to record transaction")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "redemption.request", "transaction", tx.ID,
		map[string]any{"account_id": acct.ID, "amount_minor": amount})
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
	s.audit(r.Context(), actor.IdentityID, action, "transaction", tx.ID,
		map[string]any{"amount_minor": tx.AmountMinor, "type": tx.Type})

	updated, _ := s.store.GetTransaction(r.Context(), actor.TenantID, tx.ID)
	writeJSON(w, http.StatusOK, updated)
}
