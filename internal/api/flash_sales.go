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

type flashSaleRequest struct {
	DiscountType string `json:"discount_type"` // "percent" | "fixed"
	PercentOff   *int   `json:"percent_off,omitempty"`
	AmountOff    string `json:"amount_off,omitempty"` // coin string, e.g. "0.25"
	StartsAt     string `json:"starts_at,omitempty"`  // optional; blank means "now"
	EndsAt       string `json:"ends_at"`
}

func (s *Server) handleListFlashSales(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	sales, err := s.store.ListFlashSales(r.Context(), actor.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to list flash sales")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flash_sales": sales})
}

// handleGetActiveFlashSale reports the sale (if any) currently in effect, and
// the price a redemption would cost right now — both portals poll this to
// show up-to-date pricing.
func (s *Server) handleGetActiveFlashSale(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	base := money.Coin(1)
	sale, err := s.store.GetActiveFlashSale(r.Context(), actor.TenantID)
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"active": false, "base_price_minor": base, "effective_price_minor": base})
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to check for a flash sale")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active": true, "flash_sale": sale,
		"base_price_minor": base, "effective_price_minor": sale.Apply(base),
	})
}

func (s *Server) handleCreateFlashSale(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	var req flashSaleRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	discountType := domain.FlashSaleDiscountType(strings.TrimSpace(req.DiscountType))
	var percentOff *int
	var amountOffMinor *int64
	switch discountType {
	case domain.FlashSalePercent:
		if req.PercentOff == nil || *req.PercentOff < 1 || *req.PercentOff > 99 {
			writeErr(w, http.StatusBadRequest, "bad_request", "percent_off must be between 1 and 99")
			return
		}
		percentOff = req.PercentOff
	case domain.FlashSaleFixed:
		amt, err := money.ParseCoins(req.AmountOff)
		if err != nil || amt <= 0 || amt >= money.Coin(1) {
			writeErr(w, http.StatusBadRequest, "bad_request", "amount_off must be a positive coin amount less than 1, e.g. \"0.25\"")
			return
		}
		amountOffMinor = &amt
	default:
		writeErr(w, http.StatusBadRequest, "bad_request", `discount_type must be "percent" or "fixed"`)
		return
	}

	now := time.Now()
	startsAt := now
	if strings.TrimSpace(req.StartsAt) != "" {
		t, err := parseOccurred(req.StartsAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "starts_at must be a date (YYYY-MM-DD) or RFC3339 timestamp")
			return
		}
		startsAt = *t
	}
	endsAt, err := parseOccurred(req.EndsAt)
	if err != nil || endsAt == nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "ends_at must be a date (YYYY-MM-DD) or RFC3339 timestamp")
		return
	}
	if !endsAt.After(startsAt) {
		writeErr(w, http.StatusBadRequest, "bad_request", "ends_at must be after starts_at")
		return
	}
	if !endsAt.After(now) {
		writeErr(w, http.StatusBadRequest, "bad_request", "ends_at must be in the future")
		return
	}

	sale := &domain.FlashSale{
		ID: uuid.NewString(), TenantID: actor.TenantID, DiscountType: discountType,
		PercentOff: percentOff, AmountOffMinor: amountOffMinor,
		StartsAt: startsAt, EndsAt: *endsAt, CreatedBy: actor.IdentityID,
	}
	if err := s.store.CreateFlashSale(r.Context(), sale); err == store.ErrConflict {
		writeErr(w, http.StatusConflict, "conflict", "overlaps an existing scheduled or active flash sale")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to create flash sale")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "flash_sale.create", "flash_sale", sale.ID,
		map[string]any{"discount_type": sale.DiscountType, "percent_off": sale.PercentOff, "amount_off_minor": sale.AmountOffMinor, "starts_at": sale.StartsAt, "ends_at": sale.EndsAt})

	// A sale that starts immediately notifies right away, same as posting a
	// bounty. One scheduled for later is left for the periodic sweep to catch
	// once its window actually opens.
	if !sale.StartsAt.After(now) {
		s.notifier.NotifyHolders(r.Context(), actor.TenantID, domain.NotifyFlashSaleStarted,
			"Flash sale!", sale.Describe()+" until "+sale.EndsAt.Local().Format("Jan 2 3:04 PM")+".",
			map[string]any{"flash_sale_id": sale.ID})
		_ = s.store.MarkFlashSaleStartNotified(r.Context(), actor.TenantID, sale.ID)
	}
	writeJSON(w, http.StatusCreated, sale)
}

func (s *Server) handleCancelFlashSale(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	if err := s.store.CancelFlashSale(r.Context(), actor.TenantID, id); err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "flash sale not found, already ended, or already canceled")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "failed to cancel flash sale")
		return
	}
	s.audit(r.Context(), actor.IdentityID, "flash_sale.cancel", "flash_sale", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "canceled": true})
}
