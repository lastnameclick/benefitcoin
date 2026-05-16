package api

import (
	"net/http"
	"time"

	"cpal/internal/auth"
)

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	events, err := s.store.ListAudit(r.Context(), actor.TenantID, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list audit events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func nowUTC() time.Time { return time.Now().UTC() }
