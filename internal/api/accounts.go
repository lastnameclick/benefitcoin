package api

import (
	"net/http"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
)

// accountView is the API representation of an account, including a balance.
type accountView struct {
	domain.Account
	Balance *balanceView `json:"balance,omitempty"`
}

// balanceView presents balances in both minor units and formatted coin strings.
type balanceView struct {
	CurrentMinor          int64  `json:"current_minor"`
	AvailableMinor        int64  `json:"available_minor"`
	AwaitingApprovalMinor int64  `json:"awaiting_approval_minor"`
	Current               string `json:"current"`
	Available             string `json:"available"`
	AwaitingApproval      string `json:"awaiting_approval"`
	Currency              string `json:"currency"`
}

func toBalanceView(b domain.Balance) balanceView {
	return balanceView{
		CurrentMinor:          b.Current(),
		AvailableMinor:        b.Available(),
		AwaitingApprovalMinor: b.AwaitingApproval(),
		Current:               money.Format(b.Current()),
		Available:             money.Format(b.Available()),
		AwaitingApproval:      money.Format(b.AwaitingApproval()),
		Currency:              money.Currency,
	}
}

func (s *Server) accountViewWithBalance(r *http.Request, a domain.Account) accountView {
	view := accountView{Account: a}
	if bal, err := s.ledger.Balance(a.TBAccountID); err == nil {
		bv := toBalanceView(bal)
		view.Balance = &bv
	}
	return view
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var (
		accts []domain.Account
		err   error
	)
	if claims.Role == domain.RoleOperator {
		accts, err = s.store.ListCustomerAccounts(r.Context(), claims.TenantID)
	} else {
		accts, err = s.store.ListAccountsByCustomer(r.Context(), claims.TenantID, claims.CustomerID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list accounts")
		return
	}
	views := make([]accountView, 0, len(accts))
	for _, a := range accts {
		views = append(views, s.accountViewWithBalance(r, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": views})
}

// loadAccountAuthorized fetches an account and enforces that holders may only
// touch their own. Writes the appropriate error and returns ok=false on failure.
func (s *Server) loadAccountAuthorized(w http.ResponseWriter, r *http.Request) (domain.Account, bool) {
	claims, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	acct, err := s.store.GetAccount(r.Context(), claims.TenantID, id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "account not found")
		return domain.Account{}, false
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load account")
		return domain.Account{}, false
	}
	if claims.Role != domain.RoleOperator {
		if acct.CustomerID == nil || *acct.CustomerID != claims.CustomerID {
			writeErr(w, http.StatusForbidden, "forbidden", "not your account")
			return domain.Account{}, false
		}
	}
	return acct, true
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.accountViewWithBalance(r, acct))
}

func (s *Server) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	bal, err := s.ledger.Balance(acct.TBAccountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to read balance")
		return
	}
	writeJSON(w, http.StatusOK, toBalanceView(bal))
}

func (s *Server) handleAccountTransactions(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.loadAccountAuthorized(w, r)
	if !ok {
		return
	}
	claims, _ := auth.FromContext(r.Context())
	txs, err := s.store.ListTransactions(r.Context(), claims.TenantID, "", acct.ID, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list transactions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": txs})
}
