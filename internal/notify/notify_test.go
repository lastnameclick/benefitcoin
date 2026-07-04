package notify

import (
	"context"
	"testing"
	"time"

	"cpal/internal/config"
	"cpal/internal/domain"
)

func TestSubscribeBroadcast(t *testing.T) {
	svc := New(nil, config.PushConfig{})
	events, unsubscribe := svc.Subscribe("identity-1")
	defer unsubscribe()

	want := Event{Notification: domain.Notification{ID: "n1", IdentityID: "identity-1", Title: "hi"}}
	svc.broadcast("identity-1", want)

	select {
	case got := <-events:
		if got.Notification.ID != want.Notification.ID {
			t.Fatalf("got notification %q, want %q", got.Notification.ID, want.Notification.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}

func TestBroadcastIgnoresOtherIdentities(t *testing.T) {
	svc := New(nil, config.PushConfig{})
	events, unsubscribe := svc.Subscribe("identity-1")
	defer unsubscribe()

	svc.broadcast("identity-2", Event{Notification: domain.Notification{ID: "n1"}})

	select {
	case got := <-events:
		t.Fatalf("expected no event for identity-1, got %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no event delivered
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	svc := New(nil, config.PushConfig{})
	events, unsubscribe := svc.Subscribe("identity-1")
	unsubscribe()

	svc.broadcast("identity-1", Event{Notification: domain.Notification{ID: "n1"}})

	if _, ok := <-events; ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestBroadcastDropsRatherThanBlocksSlowConsumer(t *testing.T) {
	svc := New(nil, config.PushConfig{})
	_, unsubscribe := svc.Subscribe("identity-1") // never drained
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { // far more than the channel buffer
			svc.broadcast("identity-1", Event{Notification: domain.Notification{ID: "n"}})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a slow consumer instead of dropping")
	}
}

// sendPush must be a safe no-op when Web Push isn't configured — it must
// return before touching the store, since Notify is called with store access
// unavailable in this unit test (nil store).
func TestSendPushNoopWhenNotConfigured(t *testing.T) {
	svc := New(nil, config.PushConfig{})
	svc.sendPush(context.Background(), domain.Notification{ID: "n1", TenantID: "t1", IdentityID: "i1"})
}

func TestPushConfigConfigured(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.PushConfig
		want bool
	}{
		{"empty", config.PushConfig{}, false},
		{"missing private key", config.PushConfig{PublicKey: "pub"}, false},
		{"missing public key", config.PushConfig{PrivateKey: "priv"}, false},
		{"fully configured", config.PushConfig{PublicKey: "pub", PrivateKey: "priv"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.Configured(); got != c.want {
				t.Errorf("Configured() = %v, want %v", got, c.want)
			}
		})
	}
}
