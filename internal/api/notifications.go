package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	notifications, err := s.store.ListNotifications(r.Context(), claims.TenantID, claims.IdentityID, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list notifications")
		return
	}
	unread, err := s.store.CountUnreadNotifications(r.Context(), claims.TenantID, claims.IdentityID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to count unread notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifications, "unread_count": unread})
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	if err := s.store.MarkNotificationRead(r.Context(), claims.TenantID, claims.IdentityID, id); err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "notification not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to mark notification read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	if err := s.store.MarkAllNotificationsRead(r.Context(), claims.TenantID, claims.IdentityID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to mark notifications read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// streamTicketTTL is intentionally short: a ticket is exchanged for an SSE
// connection within seconds of being minted, and single-use, so it never
// needs to outlive the immediate connect handshake.
const streamTicketTTL = 2 * time.Minute

// handleNotificationStreamToken mints a short-lived, single-use ticket that
// authenticates the SSE connection below. EventSource can't set an
// Authorization header, so the long-lived access/refresh tokens never need to
// touch a URL or a server log.
func (s *Server) handleNotificationStreamToken(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to mint stream ticket")
		return
	}
	ticket := hex.EncodeToString(buf)
	s.putStreamTicket(ticket, claims)
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket, "expires_in": int(streamTicketTTL.Seconds())})
}

// handleNotificationStream is a Server-Sent Events endpoint: it holds the
// connection open and pushes each new notification for the ticket's identity
// as it happens. Deliberately unauthenticated by the normal Bearer
// middleware — see handleNotificationStreamToken.
func (s *Server) handleNotificationStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.takeStreamTicket(r.URL.Query().Get("ticket"))
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "missing or expired stream ticket")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "internal", "streaming not supported")
		return
	}

	events, unsubscribe := s.notifier.Subscribe(claims.IdentityID)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case ev, ok := <-events:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev.Notification)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (s *Server) handleVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.cfg.Push.PublicKey})
}

type pushSubscribeRequest struct {
	Endpoint       string   `json:"endpoint"`
	ExpirationTime *float64 `json:"expirationTime"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var req pushSubscribeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "endpoint and keys.p256dh/keys.auth are required")
		return
	}
	sub := &domain.PushSubscription{
		ID: uuid.NewString(), TenantID: claims.TenantID, IdentityID: claims.IdentityID,
		Endpoint: req.Endpoint, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth,
	}
	if err := s.store.UpsertPushSubscription(r.Context(), sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to save push subscription")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var req pushUnsubscribeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.DeletePushSubscription(r.Context(), claims.TenantID, claims.IdentityID, req.Endpoint); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to remove push subscription")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
