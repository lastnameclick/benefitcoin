package api

import (
	"net/http"
	"strings"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createCustomerRequest struct {
	Type        string `json:"type"` // "operator" or "holder"
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type customerResponse struct {
	domain.Customer
	Username string        `json:"username"`
	Account  *accountView  `json:"account,omitempty"`
	Accounts []accountView `json:"accounts,omitempty"`
}

// handleCreateCustomer onboards a new party: creates the customer + login, and
// (for holders) opens a deposit account in the ledger.
func (s *Server) handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	var req createCustomerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	ctype := domain.CustomerType(req.Type)
	if ctype != domain.CustomerOperator && ctype != domain.CustomerHolder {
		writeErr(w, http.StatusBadRequest, "bad_request", "type must be 'operator' or 'holder'")
		return
	}
	if req.Username == "" || len(req.Password) < 6 || strings.TrimSpace(req.DisplayName) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "display_name, username and a 6+ char password are required")
		return
	}
	if _, err := s.store.GetIdentityByUsername(r.Context(), req.Username); err == nil {
		writeErr(w, http.StatusConflict, "username_taken", "that username is already in use")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to hash password")
		return
	}
	cust := &domain.Customer{ID: uuid.NewString(), TenantID: actor.TenantID, Type: ctype, DisplayName: strings.TrimSpace(req.DisplayName)}
	if err := s.store.CreateCustomer(r.Context(), cust); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create customer")
		return
	}
	role := domain.RoleHolder
	if ctype == domain.CustomerOperator {
		role = domain.RoleOperator
	}
	identity := &domain.Identity{
		ID: uuid.NewString(), TenantID: actor.TenantID, CustomerID: cust.ID, Username: req.Username, PasswordHash: hash, Role: role,
	}
	if err := s.store.CreateIdentity(r.Context(), identity); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create login")
		return
	}

	resp := customerResponse{Customer: *cust, Username: req.Username}

	// Holders get a deposit account opened in the ledger.
	if ctype == domain.CustomerHolder {
		acct, err := s.openCustomerAccount(r, cust)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "failed to open account: "+err.Error())
			return
		}
		view := s.accountViewWithBalance(r, acct)
		resp.Account = &view
	}

	s.audit(r.Context(), actor.IdentityID, "customer.create", "customer", cust.ID,
		map[string]any{"type": req.Type, "username": req.Username})
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) openCustomerAccount(r *http.Request, cust *domain.Customer) (domain.Account, error) {
	tbID, err := s.ledger.OpenAccount()
	if err != nil {
		return domain.Account{}, err
	}
	custID := cust.ID
	acct := domain.Account{
		ID: uuid.NewString(), TenantID: cust.TenantID, CustomerID: &custID, Kind: domain.AccountCustomer,
		TBAccountID: tbID, Name: cust.DisplayName + " Wallet",
	}
	if err := s.store.CreateAccount(r.Context(), &acct); err != nil {
		return domain.Account{}, err
	}
	return acct, nil
}

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	custs, err := s.store.ListCustomers(r.Context(), actor.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list customers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": custs})
}

func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	cust, err := s.store.GetCustomer(r.Context(), actor.TenantID, id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "customer not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load customer")
		return
	}
	accts, _ := s.store.ListAccountsByCustomer(r.Context(), actor.TenantID, id)
	resp := customerResponse{Customer: cust}
	for _, a := range accts {
		resp.Accounts = append(resp.Accounts, s.accountViewWithBalance(r, a))
	}
	writeJSON(w, http.StatusOK, resp)
}
