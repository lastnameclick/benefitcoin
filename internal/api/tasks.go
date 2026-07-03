package api

import (
	"net/http"
	"strings"
	"time"

	"cpal/internal/auth"
	"cpal/internal/domain"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type taskRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`                // coin string, e.g. "0.15"
	Active      *bool  `json:"active,omitempty"`     // PATCH only
	IsBounty    bool   `json:"is_bounty,omitempty"`  // one-time: first holder to claim it wins
	ExpiresAt   string `json:"expires_at,omitempty"` // bounty only: "YYYY-MM-DD" or RFC3339; empty means it never expires
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	activeOnly := claims.Role != domain.RoleOperator // holders only see active tasks
	tasks, err := s.store.ListTasks(r.Context(), claims.TenantID, activeOnly)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list tasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	var req taskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	valueMinor, err := money.ParseCoins(req.Value)
	if err != nil || valueMinor <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "value must be a positive coin amount, e.g. \"0.15\"")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		if !req.IsBounty {
			writeErr(w, http.StatusBadRequest, "bad_request", "expires_at only applies to bounties")
			return
		}
		t, err := parseOccurred(req.ExpiresAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "expires_at must be a date (YYYY-MM-DD) or RFC3339 timestamp")
			return
		}
		if t == nil || !t.After(time.Now()) {
			writeErr(w, http.StatusBadRequest, "bad_request", "expires_at must be in the future")
			return
		}
		expiresAt = t
	}
	task := &domain.Task{
		ID: uuid.NewString(), TenantID: actor.TenantID, Name: strings.TrimSpace(req.Name),
		Description: req.Description, ValueMinor: valueMinor, Active: active, IsBounty: req.IsBounty,
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreateTask(r.Context(), task); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create task")
		return
	}
	action := "task.create"
	if task.IsBounty {
		action = "bounty.create"
	}
	s.audit(r.Context(), actor.IdentityID, action, "task", task.ID,
		map[string]any{"name": task.Name, "value_minor": task.ValueMinor, "is_bounty": task.IsBounty, "expires_at": expiresAt})
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	task, err := s.store.GetTask(r.Context(), actor.TenantID, id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "task not found")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to load task")
		return
	}
	var req taskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		task.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		task.Description = req.Description
	}
	if req.Value != "" {
		valueMinor, err := money.ParseCoins(req.Value)
		if err != nil || valueMinor <= 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "value must be a positive coin amount")
			return
		}
		task.ValueMinor = valueMinor
	}
	if req.Active != nil {
		task.Active = *req.Active
	}
	if err := s.store.UpdateTask(r.Context(), &task); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to update task")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "task.update", "task", task.ID, nil)
	writeJSON(w, http.StatusOK, task)
}
