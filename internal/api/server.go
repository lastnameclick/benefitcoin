// Package api wires the HTTP layer: routing, middleware, and handlers that
// orchestrate the Postgres store and the TigerBeetle ledger.
package api

import (
	"context"
	"net/http"
	"time"

	"cpal/internal/auth"
	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	ledger *ledger.Ledger
	auth   *auth.Manager
}

func NewServer(cfg config.Config, st *store.Store, lg *ledger.Ledger, am *auth.Manager) *Server {
	return &Server{cfg: cfg, store: st, ledger: lg, auth: am}
}

// loadTenant fetches the caller's household. Writes a 500 and returns ok=false
// on failure.
func (s *Server) loadTenant(w http.ResponseWriter, r *http.Request) (domain.Tenant, bool) {
	claims, _ := auth.FromContext(r.Context())
	t, err := s.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load household")
		return domain.Tenant{}, false
	}
	return t, true
}

// Routes builds the HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(s.cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	r.Route("/api/v1", func(r chi.Router) {
		// Public white-label branding (product/site/coin names) for the SPA.
		r.Get("/config", s.handleConfig)

		// Public auth endpoints.
		r.Post("/auth/signup", s.handleSignup)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/refresh", s.handleRefresh)
		r.Post("/auth/logout", s.handleLogout)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(s.auth.Middleware(writeErr))
			r.Use(s.idempotency) // no-op for GETs / requests without the header

			r.Get("/me", s.handleMe)

			// Accounts (holders see their own; operators see all).
			r.Get("/accounts", s.handleListAccounts)
			r.Get("/accounts/{id}", s.handleGetAccount)
			r.Get("/accounts/{id}/balance", s.handleGetBalance)
			r.Get("/accounts/{id}/transactions", s.handleAccountTransactions)
			r.Post("/accounts/{id}/earnings", s.handleCreateEarning)
			r.Post("/accounts/{id}/earnings/custom", s.handleProposeChore)
			r.Post("/accounts/{id}/redemptions", s.handleCreateRedemption)

			// Tasks (holders read active; operators manage).
			r.Get("/tasks", s.handleListTasks)

			// Operator-only.
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(domain.RoleOperator, writeErr))
				r.Post("/customers", s.handleCreateCustomer)
				r.Get("/customers", s.handleListCustomers)
				r.Get("/customers/{id}", s.handleGetCustomer)
				r.Post("/accounts/{id}/adjustments", s.handleCreateAdjustment)
				r.Post("/tasks", s.handleCreateTask)
				r.Patch("/tasks/{id}", s.handleUpdateTask)
				r.Get("/transactions", s.handleListTransactions)
				r.Post("/transactions/{id}/settle", s.handleSettle)
				r.Post("/transactions/{id}/void", s.handleVoid)
				r.Post("/transactions/{id}/adjust", s.handleAdjustTransaction)
				r.Get("/audit", s.handleListAudit)
			})
		})
	})
	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// audit records a state-changing action, scoped to the caller's household;
// failures are logged, not fatal.
func (s *Server) audit(ctx context.Context, actorID, action, entityType, entityID string, meta map[string]any) {
	claims, _ := auth.FromContext(ctx)
	_ = s.store.InsertAudit(ctx, &domain.AuditEvent{
		ID: uuid.NewString(), TenantID: claims.TenantID, ActorIdentityID: &actorID, Action: action,
		EntityType: entityType, EntityID: entityID, Metadata: meta,
	})
}
