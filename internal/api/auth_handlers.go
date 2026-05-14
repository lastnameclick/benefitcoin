package api

import (
	"net/http"
	"strings"
	"time"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/google/uuid"
)

// handleConfig returns the operator-configured white-label branding. Public so
// the SPA can render the right names before anyone logs in.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Branding)
}

type signupRequest struct {
	HouseholdName string `json:"household_name"`
	DisplayName   string `json:"display_name"`
	Email         string `json:"email"`
	Password      string `json:"password"`
}

// handleSignup provisions a new household: it opens the household's Issuance and
// Redemption GL accounts in the ledger, then creates the first operator (a
// parent) who can go on to onboard kids and co-parents. This is the self-serve
// front door — no existing login required.
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.HouseholdName = strings.TrimSpace(req.HouseholdName)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.HouseholdName == "" || req.DisplayName == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "household_name and display_name are required")
		return
	}
	if !looksLikeEmail(req.Email) {
		writeErr(w, http.StatusBadRequest, "bad_request", "a valid email is required")
		return
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "bad_request", "password must be at least 6 characters")
		return
	}
	if _, err := s.store.GetIdentityByUsername(r.Context(), req.Email); err == nil {
		writeErr(w, http.StatusConflict, "email_taken", "that email is already registered")
		return
	} else if err != store.ErrNotFound {
		writeErr(w, http.StatusInternalServerError, "internal", "signup lookup failed")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to hash password")
		return
	}

	// Open this household's own pair of general-ledger accounts.
	issuanceTB, err := s.ledger.OpenGL()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to open issuance GL: "+err.Error())
		return
	}
	redemptionTB, err := s.ledger.OpenGL()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ledger_error", "failed to open redemption GL: "+err.Error())
		return
	}

	tenant := &domain.Tenant{
		ID: uuid.NewString(), Name: req.HouseholdName,
		IssuanceAccountID: uuid.NewString(), RedemptionAccountID: uuid.NewString(),
		IssuanceTBID: issuanceTB, RedemptionTBID: redemptionTB,
	}
	if err := s.store.CreateTenant(r.Context(), tenant); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create household")
		return
	}
	glAccounts := []domain.Account{
		{ID: tenant.IssuanceAccountID, TenantID: tenant.ID, TBAccountID: issuanceTB, Currency: money.Currency, Name: "Issuance GL"},
		{ID: tenant.RedemptionAccountID, TenantID: tenant.ID, TBAccountID: redemptionTB, Currency: money.Currency, Name: "Redemption GL"},
	}
	for i := range glAccounts {
		if err := s.store.CreateInternalAccount(r.Context(), &glAccounts[i]); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "failed to record GL account")
			return
		}
	}

	cust := &domain.Customer{ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.CustomerOperator, DisplayName: req.DisplayName}
	if err := s.store.CreateCustomer(r.Context(), cust); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create operator")
		return
	}
	identity := domain.Identity{
		ID: uuid.NewString(), TenantID: tenant.ID, CustomerID: cust.ID,
		Username: req.Email, PasswordHash: hash, Role: domain.RoleOperator,
	}
	if err := s.store.CreateIdentity(r.Context(), &identity); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create login")
		return
	}

	s.issueTokens(w, r, identity, tenant.Name)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	ExpiresAt    time.Time   `json:"expires_at"`
	RefreshToken string      `json:"refresh_token"`
	Role         domain.Role `json:"role"`
	CustomerID   string      `json:"customer_id"`
	TenantID     string      `json:"tenant_id"`
	Household    string      `json:"household"`
	Username     string      `json:"username"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	username := strings.TrimSpace(strings.ToLower(req.Username))
	id, err := s.store.GetIdentityByUsername(r.Context(), username)
	if err != nil || !auth.CheckPassword(id.PasswordHash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "incorrect username or password")
		return
	}
	s.issueTokens(w, r, id, s.householdName(r, id.TenantID))
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	hash := auth.HashRefresh(req.RefreshToken)
	tok, err := s.store.GetRefreshTokenByHash(r.Context(), hash)
	if err != nil || tok.RevokedAt != nil || time.Now().After(tok.ExpiresAt) {
		writeErr(w, http.StatusUnauthorized, "invalid_token", "refresh token is invalid or expired")
		return
	}
	id, err := s.store.GetIdentity(r.Context(), tok.IdentityID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_token", "identity not found")
		return
	}
	// Rotate: revoke the used refresh token before issuing a fresh pair.
	_ = s.store.RevokeRefreshToken(r.Context(), hash)
	s.issueTokens(w, r, id, s.householdName(r, id.TenantID))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	_ = s.store.RevokeRefreshToken(r.Context(), auth.HashRefresh(req.RefreshToken))
	w.WriteHeader(http.StatusNoContent)
}

// issueTokens mints an access token + a persisted refresh token for an identity.
func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, id domain.Identity, household string) {
	claims := auth.Claims{IdentityID: id.ID, TenantID: id.TenantID, CustomerID: id.CustomerID, Username: id.Username, Role: id.Role}
	access, exp, err := s.auth.IssueAccess(claims, time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to issue token")
		return
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to issue refresh token")
		return
	}
	err = s.store.CreateRefreshToken(r.Context(), &store.RefreshToken{
		ID: uuid.NewString(), IdentityID: id.ID, TokenHash: hash,
		ExpiresAt: time.Now().Add(s.auth.RefreshTTL()),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to persist refresh token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: access, ExpiresAt: exp, RefreshToken: raw,
		Role: id.Role, CustomerID: id.CustomerID, TenantID: id.TenantID,
		Household: household, Username: id.Username,
	})
}

func (s *Server) householdName(r *http.Request, tenantID string) string {
	if t, err := s.store.GetTenant(r.Context(), tenantID); err == nil {
		return t.Name
	}
	return ""
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"identity_id": c.IdentityID,
		"tenant_id":   c.TenantID,
		"household":   s.householdName(r, c.TenantID),
		"customer_id": c.CustomerID,
		"username":    c.Username,
		"role":        c.Role,
	})
}

// looksLikeEmail is a light sanity check — real validation happens at delivery
// time, which this project doesn't do.
func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}
