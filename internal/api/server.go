// Package api wires the HTTP layer: routing, middleware, and handlers that
// orchestrate the Postgres store and the TigerBeetle ledger.
package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"cpal/internal/auth"
	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/notify"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	ledger   *ledger.Ledger
	auth     *auth.Manager
	notifier *notify.Service

	ticketMu sync.Mutex
	tickets  map[string]streamTicket // one-time SSE connect tickets, see notifications.go
}

type streamTicket struct {
	claims  auth.Claims
	expires time.Time
}

func NewServer(cfg config.Config, st *store.Store, lg *ledger.Ledger, am *auth.Manager, nf *notify.Service) *Server {
	return &Server{cfg: cfg, store: st, ledger: lg, auth: am, notifier: nf, tickets: make(map[string]streamTicket)}
}

// putStreamTicket stores a one-time SSE connect ticket, sweeping expired
// entries opportunistically so the map doesn't grow unbounded.
func (s *Server) putStreamTicket(ticket string, claims auth.Claims) {
	s.ticketMu.Lock()
	defer s.ticketMu.Unlock()
	now := time.Now()
	for k, v := range s.tickets {
		if now.After(v.expires) {
			delete(s.tickets, k)
		}
	}
	s.tickets[ticket] = streamTicket{claims: claims, expires: now.Add(streamTicketTTL)}
}

// takeStreamTicket validates and consumes a ticket — each one connects at
// most once.
func (s *Server) takeStreamTicket(ticket string) (auth.Claims, bool) {
	if ticket == "" {
		return auth.Claims{}, false
	}
	s.ticketMu.Lock()
	defer s.ticketMu.Unlock()
	t, ok := s.tickets[ticket]
	delete(s.tickets, ticket)
	if !ok || time.Now().After(t.expires) {
		return auth.Claims{}, false
	}
	return t.claims, true
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

		// The live notification stream authenticates via a short-lived ticket
		// query param (EventSource can't set an Authorization header) and
		// deliberately sits outside middleware.Timeout below — a 30s hard
		// deadline would kill every long-lived SSE connection.
		r.Get("/notifications/stream", s.handleNotificationStream)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Second))
			r.Use(s.auth.Middleware(writeErr))
			r.Use(s.idempotency) // no-op for GETs / requests without the header

			r.Get("/me", s.handleMe)

			// Notifications (in-app feed) and Web Push subscriptions.
			r.Get("/notifications", s.handleListNotifications)
			r.Post("/notifications/{id}/read", s.handleMarkNotificationRead)
			r.Post("/notifications/read-all", s.handleMarkAllNotificationsRead)
			r.Get("/notifications/stream-token", s.handleNotificationStreamToken)
			r.Get("/push/vapid-public-key", s.handleVapidPublicKey)
			r.Post("/push/subscribe", s.handlePushSubscribe)
			r.Delete("/push/subscribe", s.handlePushUnsubscribe)

			// Accounts (holders see their own; operators see all).
			r.Get("/accounts", s.handleListAccounts)
			r.Get("/accounts/{id}", s.handleGetAccount)
			r.Get("/accounts/{id}/balance", s.handleGetBalance)
			r.Get("/accounts/{id}/transactions", s.handleAccountTransactions)
			r.Post("/accounts/{id}/earnings", s.handleCreateEarning)
			r.Post("/accounts/{id}/earnings/custom", s.handleProposeChore)
			r.Post("/accounts/{id}/redemptions", s.handleCreateRedemption)

			// Charts (holders see their own account; operators can view any).
			r.Get("/accounts/{id}/charts/balance-history", s.handleBalanceHistory)
			r.Get("/accounts/{id}/charts/earn-redeem", s.handleEarnRedeem)
			r.Get("/accounts/{id}/charts/redemption-frequency", s.handleRedemptionFrequency)
			r.Get("/accounts/{id}/charts/task-leaderboard", s.handleTaskLeaderboard)

			// Monthly PDF statements: on-demand generation and the always-available
			// in-app Inbox (works with zero SMTP configuration).
			r.Get("/accounts/{id}/statement.pdf", s.handleDownloadStatement)
			r.Get("/accounts/{id}/inbox", s.handleListInbox)
			r.Get("/accounts/{id}/inbox/{statementId}/pdf", s.handleDownloadInboxStatement)

			// Tasks (holders read active; operators manage).
			r.Get("/tasks", s.handleListTasks)

			// Flash sales (holders read the currently active one; operators manage).
			r.Get("/flash-sales/active", s.handleGetActiveFlashSale)

			// Operator-only.
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(domain.RoleOperator, writeErr))
				r.Post("/customers", s.handleCreateCustomer)
				r.Get("/customers", s.handleListCustomers)
				r.Get("/customers/{id}", s.handleGetCustomer)
				r.Post("/accounts/{id}/adjustments", s.handleCreateAdjustment)
				r.Post("/tasks", s.handleCreateTask)
				r.Patch("/tasks/{id}", s.handleUpdateTask)
				r.Get("/flash-sales", s.handleListFlashSales)
				r.Post("/flash-sales", s.handleCreateFlashSale)
				r.Post("/flash-sales/{id}/cancel", s.handleCancelFlashSale)
				r.Get("/transactions", s.handleListTransactions)
				r.Post("/transactions/{id}/settle", s.handleSettle)
				r.Post("/transactions/{id}/void", s.handleVoid)
				r.Post("/transactions/{id}/adjust", s.handleAdjustTransaction)
				r.Get("/audit", s.handleListAudit)
				r.Get("/tenant/charts/task-leaderboard", s.handleHouseholdLeaderboard)
				r.Get("/tenant/charts/household-overview", s.handleHouseholdOverview)
			})
		})
	})
	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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
