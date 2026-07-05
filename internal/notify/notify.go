// Package notify is the real-time notification layer: it records events to
// an in-app feed, fans them out to any open SSE connection, and best-effort
// delivers them as Web Push — mirroring internal/mail's philosophy that the
// in-app channel always works, and the push channel is a bonus on top of it.
package notify

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/store"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
)

// Event is one notification delivered to a live SSE subscriber.
type Event struct {
	Notification domain.Notification `json:"notification"`
}

// Service creates notifications, fans them out to live listeners, and
// best-effort delivers Web Push. Safe for concurrent use.
type Service struct {
	store *store.Store
	push  config.PushConfig

	mu   sync.Mutex
	subs map[string][]chan Event // identityID -> live SSE listeners
}

func New(st *store.Store, push config.PushConfig) *Service {
	return &Service{store: st, push: push, subs: make(map[string][]chan Event)}
}

// Subscribe registers a live listener for one identity's notifications. The
// caller must invoke the returned unsubscribe func when its connection ends.
func (s *Service) Subscribe(identityID string) (<-chan Event, func()) {
	ch := make(chan Event, 8)
	s.mu.Lock()
	s.subs[identityID] = append(s.subs[identityID], ch)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		chans := s.subs[identityID]
		for i, c := range chans {
			if c == ch {
				s.subs[identityID] = append(chans[:i], chans[i+1:]...)
				break
			}
		}
		close(ch)
	}
}

func (s *Service) broadcast(identityID string, ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs[identityID] {
		select {
		case ch <- ev:
		default: // slow consumer — drop rather than block the caller
		}
	}
}

// Notify records a notification for one identity, broadcasts it to any open
// SSE stream, and best-effort sends a Web Push. It never returns an error:
// failures are logged, since a notification must never fail the
// state-changing request that triggered it.
func (s *Service) Notify(ctx context.Context, tenantID, identityID string, typ domain.NotificationType, title, body string, data map[string]any) {
	n := &domain.Notification{
		ID: uuid.NewString(), TenantID: tenantID, IdentityID: identityID,
		Type: typ, Title: title, Body: body, Data: data,
	}
	if err := s.store.InsertNotification(ctx, n); err != nil {
		log.Printf("notify: insert failed: %v", err)
		return
	}
	s.broadcast(identityID, Event{Notification: *n})
	s.sendPush(ctx, *n)
}

// NotifyOperators notifies every operator identity in a household.
func (s *Service) NotifyOperators(ctx context.Context, tenantID string, typ domain.NotificationType, title, body string, data map[string]any) {
	ids, err := s.store.ListIdentitiesByRole(ctx, tenantID, domain.RoleOperator)
	if err != nil {
		log.Printf("notify: list operators failed: %v", err)
		return
	}
	for _, id := range ids {
		s.Notify(ctx, tenantID, id.ID, typ, title, body, data)
	}
}

// NotifyHolders notifies every holder identity in a household — for
// household-wide alerts like a new bounty being posted.
func (s *Service) NotifyHolders(ctx context.Context, tenantID string, typ domain.NotificationType, title, body string, data map[string]any) {
	ids, err := s.store.ListIdentitiesByRole(ctx, tenantID, domain.RoleHolder)
	if err != nil {
		log.Printf("notify: list holders failed: %v", err)
		return
	}
	for _, id := range ids {
		s.Notify(ctx, tenantID, id.ID, typ, title, body, data)
	}
}

// NotifyCustomer notifies the identity belonging to one customer (e.g. the
// holder who owns an account or who claimed a bounty).
func (s *Service) NotifyCustomer(ctx context.Context, tenantID, customerID string, typ domain.NotificationType, title, body string, data map[string]any) {
	identity, err := s.store.GetIdentityByCustomer(ctx, tenantID, customerID)
	if err != nil {
		log.Printf("notify: resolve customer identity failed: %v", err)
		return
	}
	s.Notify(ctx, tenantID, identity.ID, typ, title, body, data)
}

func (s *Service) sendPush(ctx context.Context, n domain.Notification) {
	if !s.push.Configured() {
		return
	}
	subs, err := s.store.ListPushSubscriptions(ctx, n.TenantID, n.IdentityID)
	if err != nil || len(subs) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"title": n.Title, "body": n.Body, "type": n.Type, "data": n.Data,
	})
	if err != nil {
		return
	}
	for _, sub := range subs {
		resp, err := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
		}, &webpush.Options{
			Subscriber:      s.push.Subject,
			VAPIDPublicKey:  s.push.PublicKey,
			VAPIDPrivateKey: s.push.PrivateKey,
			TTL:             int(24 * time.Hour / time.Second),
		})
		if err != nil {
			log.Printf("notify: push send failed: %v", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			_ = s.store.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint)
		}
	}
}

// bountyExpiryWindow is how far ahead of a bounty's deadline the "expiring
// soon" alert fires.
const bountyExpiryWindow = 15 * time.Minute

// SweepBounties runs one pass of the periodic bounty sweep across every
// active household: it warns holders about unclaimed bounties expiring soon,
// and retires + reports on bounties whose deadline has already passed.
// Meant to be called on a ticker from cmd/api's main loop.
func (s *Service) SweepBounties(ctx context.Context) {
	tenants, err := s.store.ListActiveTenants(ctx)
	if err != nil {
		log.Printf("notify: sweep: list tenants failed: %v", err)
		return
	}
	for _, tenant := range tenants {
		soon, err := s.store.ListBountiesExpiringSoon(ctx, tenant.ID, bountyExpiryWindow)
		if err != nil {
			log.Printf("notify: sweep: list expiring-soon failed for tenant %s: %v", tenant.ID, err)
		}
		for _, task := range soon {
			s.NotifyHolders(ctx, tenant.ID, domain.NotifyBountyExpiringSoon,
				"Bounty expiring soon", task.Name+" expires soon — claim it before it's gone!",
				map[string]any{"task_id": task.ID})
			if err := s.store.MarkBountyExpiringNotified(ctx, tenant.ID, task.ID); err != nil {
				log.Printf("notify: sweep: mark notified failed for task %s: %v", task.ID, err)
			}
		}

		expired, err := s.store.RetireExpiredBountiesDetailed(ctx, tenant.ID)
		if err != nil {
			log.Printf("notify: sweep: retire expired failed for tenant %s: %v", tenant.ID, err)
			continue
		}
		for _, task := range expired {
			if task.ClaimedBy != nil {
				continue // already claimed — its pending transaction is unaffected by the task's retirement
			}
			s.NotifyOperators(ctx, tenant.ID, domain.NotifyBountyExpired,
				"Bounty expired unclaimed", task.Name+" expired without anyone claiming it.",
				map[string]any{"task_id": task.ID})
		}
	}
}
