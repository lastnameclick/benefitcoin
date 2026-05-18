package api_test

// End-to-end test of the full stack (Postgres + TigerBeetle + HTTP API).
// Requires `make up` and is gated behind CPAL_INTEGRATION=1 so plain unit-test
// runs don't need infra. `make test-integration` sets it.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cpal/internal/api"
	"cpal/internal/auth"
	"cpal/internal/config"
	"cpal/internal/ledger"
	"cpal/internal/store"
)

type apiClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *apiClient) do(method, path, token string, body any, idemKey string) (int, map[string]any) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func login(c *apiClient, user, pass string) string {
	st, body := c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"username": user, "password": pass}, "")
	if st != 200 {
		c.t.Fatalf("login %s failed: %d %v", user, st, body)
	}
	return body["access_token"].(string)
}

func balanceOf(c *apiClient, token, acctID string) (current, available, awaiting int64) {
	st, body := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/balance", token, nil, "")
	if st != 200 {
		c.t.Fatalf("balance failed: %d %v", st, body)
	}
	return int64(body["current_minor"].(float64)),
		int64(body["available_minor"].(float64)),
		int64(body["awaiting_approval_minor"].(float64))
}

func TestEndToEnd(t *testing.T) {
	if os.Getenv("CPAL_INTEGRATION") == "" {
		t.Skip("set CPAL_INTEGRATION=1 (and run `make up`) to run the integration test")
	}
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	lg, err := ledger.Connect(cfg.TBClusterID, []string{cfg.TBAddress})
	if err != nil {
		t.Fatalf("tigerbeetle: %v", err)
	}
	defer lg.Close()

	am := auth.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	srv := api.NewServer(cfg, st, lg, am)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	c := &apiClient{t: t, base: ts.URL, http: ts.Client()}

	uniq := time.Now().UnixNano()
	idem := func(s string) string { return fmt.Sprintf("%s-%d", s, uniq) }

	// Self-serve signup provisions a household and its first operator (parent).
	opEmail := fmt.Sprintf("parent-%d@example.com", uniq)
	stSU, suBody := c.do(http.MethodPost, "/api/v1/auth/signup", "", map[string]string{
		"household_name": "Test Household", "display_name": "Parent",
		"email": opEmail, "password": "parentpass",
	}, "")
	if stSU != 200 {
		t.Fatalf("signup: %d %v", stSU, suBody)
	}
	opTok := suBody["access_token"].(string)

	// Onboard a holder (kid) — opens a deposit account.
	kidUser := fmt.Sprintf("kid-%d", uniq)
	st1, body := c.do(http.MethodPost, "/api/v1/customers", opTok, map[string]string{
		"type": "holder", "display_name": "Kiddo", "username": kidUser, "password": "kidpass",
	}, idem("cust"))
	if st1 != 201 {
		t.Fatalf("create customer: %d %v", st1, body)
	}
	acct := body["account"].(map[string]any)
	acctID := acct["id"].(string)
	kidTok := login(c, kidUser, "kidpass")

	// Operator defines two tasks.
	_, tBody := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]string{"name": "Trash", "value": "0.15"}, idem("task-trash"))
	trashID := tBody["id"].(string)
	_, tBody2 := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]string{"name": "Mow lawn", "value": "1"}, idem("task-mow"))
	mowID := tBody2["id"].(string)

	// Kid submits the trash task -> earn hold (pending; not yet spendable).
	stE, eBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings", kidTok, map[string]string{"task_id": trashID}, idem("earn-trash"))
	if stE != 201 {
		t.Fatalf("earning: %d %v", stE, eBody)
	}
	earnTxID := eBody["id"].(string)
	if cur, _, await := balanceOf(c, kidTok, acctID); cur != 0 || await != 150 {
		t.Fatalf("after earn hold want current=0 awaiting=150, got current=%d awaiting=%d", cur, await)
	}

	// Operator settles -> coins become real and spendable.
	if stS, sBody := c.do(http.MethodPost, "/api/v1/transactions/"+earnTxID+"/settle", opTok, nil, idem("settle-trash")); stS != 200 {
		t.Fatalf("settle: %d %v", stS, sBody)
	}
	if cur, avail, await := balanceOf(c, kidTok, acctID); cur != 150 || avail != 150 || await != 0 {
		t.Fatalf("after settle want 150/150/0, got %d/%d/%d", cur, avail, await)
	}

	// Not enough yet to redeem a 1-coin reward.
	if stR, _ := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/redemptions", kidTok, nil, idem("redeem-early")); stR != http.StatusConflict {
		t.Fatalf("expected 409 insufficient_funds, got %d", stR)
	}

	// Earn a full coin via the mow task and settle it (balance -> 1150).
	_, mBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings", kidTok, map[string]string{"task_id": mowID}, idem("earn-mow"))
	mowTxID := mBody["id"].(string)
	c.do(http.MethodPost, "/api/v1/transactions/"+mowTxID+"/settle", opTok, nil, idem("settle-mow"))
	if cur, _, _ := balanceOf(c, kidTok, acctID); cur != 1150 {
		t.Fatalf("after mow settle want current=1150, got %d", cur)
	}

	// Redeem a reward -> hold reserves 1 coin (available drops, current unchanged).
	stRD, rdBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/redemptions", kidTok, nil, idem("redeem"))
	if stRD != 201 {
		t.Fatalf("redemption: %d %v", stRD, rdBody)
	}
	redeemTxID := rdBody["id"].(string)
	if cur, avail, _ := balanceOf(c, kidTok, acctID); cur != 1150 || avail != 150 {
		t.Fatalf("after redeem hold want current=1150 available=150, got %d/%d", cur, avail)
	}

	// Operator settles the redemption -> coin is spent.
	c.do(http.MethodPost, "/api/v1/transactions/"+redeemTxID+"/settle", opTok, nil, idem("settle-redeem"))
	if cur, avail, _ := balanceOf(c, kidTok, acctID); cur != 150 || avail != 150 {
		t.Fatalf("after redeem settle want current=150 available=150, got %d/%d", cur, avail)
	}

	// Idempotency: replaying the trash-earn POST returns the SAME transaction.
	stReplay, replayBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings", kidTok, map[string]string{"task_id": trashID}, idem("earn-trash"))
	if stReplay != 201 || replayBody["id"].(string) != earnTxID {
		t.Fatalf("idempotent replay mismatch: status=%d id=%v (want %s)", stReplay, replayBody["id"], earnTxID)
	}

	// Void path: a new earn hold that the operator rejects leaves balance unchanged.
	_, vBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings", kidTok, map[string]string{"task_id": trashID}, idem("earn-void"))
	voidTxID := vBody["id"].(string)
	if stV, vbody := c.do(http.MethodPost, "/api/v1/transactions/"+voidTxID+"/void", opTok, nil, idem("void")); stV != 200 {
		t.Fatalf("void: %d %v", stV, vbody)
	}
	if cur, avail, await := balanceOf(c, kidTok, acctID); cur != 150 || avail != 150 || await != 0 {
		t.Fatalf("after void want 150/150/0, got %d/%d/%d", cur, avail, await)
	}

	// Operator manual adjustments (credit/debit with metadata). Balance is 150 here.
	stC, cBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/adjustments", opTok, map[string]any{
		"direction": "credit", "amount": "0.25", "reason": "Birthday bonus",
		"occurred_at": "2026-06-20", "details": map[string]any{"note": "from grandma"},
	}, idem("adj-credit"))
	if stC != 201 {
		t.Fatalf("adjustment credit: %d %v", stC, cBody)
	}
	if cBody["type"].(string) != "adjust_credit" || cBody["status"].(string) != "settled" {
		t.Fatalf("adjustment should be a settled adjust_credit, got %v/%v", cBody["type"], cBody["status"])
	}
	adjID := cBody["id"].(string)
	if cur, _, _ := balanceOf(c, opTok, acctID); cur != 400 {
		t.Fatalf("after +0.25 credit want current=400, got %d", cur)
	}

	// Debit subtracts immediately.
	if stD, dBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/adjustments", opTok, map[string]any{
		"direction": "debit", "amount": "0.10", "reason": "Lost library book",
	}, idem("adj-debit")); stD != 201 {
		t.Fatalf("adjustment debit: %d %v", stD, dBody)
	}
	if cur, _, _ := balanceOf(c, opTok, acctID); cur != 300 {
		t.Fatalf("after -0.10 debit want current=300, got %d", cur)
	}

	// Over-debit is rejected by the ledger.
	if stO, _ := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/adjustments", opTok, map[string]any{
		"direction": "debit", "amount": "100", "reason": "too much",
	}, idem("adj-over")); stO != http.StatusConflict {
		t.Fatalf("expected 409 for over-debit, got %d", stO)
	}

	// Metadata round-trips through the DB: re-fetch and confirm reason/date/details.
	_, txList := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/transactions", opTok, nil, "")
	txs := txList["transactions"].([]any)
	var found map[string]any
	for _, raw := range txs {
		m := raw.(map[string]any)
		if m["id"] == adjID {
			found = m
		}
	}
	if found == nil {
		t.Fatal("adjustment not found in account transactions")
	}
	if found["memo"] != "Birthday bonus" || found["effective_at"] == nil {
		t.Fatalf("adjustment metadata not persisted: memo=%v effective_at=%v", found["memo"], found["effective_at"])
	}
	if d, ok := found["details"].(map[string]any); !ok || d["note"] != "from grandma" {
		t.Fatalf("adjustment details not persisted: %v", found["details"])
	}

	// Holders cannot post adjustments (operator-only route).
	if stF, _ := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/adjustments", kidTok, map[string]any{
		"direction": "credit", "amount": "5", "reason": "hax",
	}, idem("kid-adj")); stF != http.StatusForbidden {
		t.Fatalf("expected 403 for holder adjustment, got %d", stF)
	}

	// A holder must not settle (operator-only).
	if stF, _ := c.do(http.MethodPost, "/api/v1/transactions/"+earnTxID+"/settle", kidTok, nil, idem("kid-settle")); stF != http.StatusForbidden {
		t.Fatalf("expected 403 for holder settle, got %d", stF)
	}

	// Tenant isolation: a second household must not see or touch the first's data.
	otherEmail := fmt.Sprintf("other-%d@example.com", uniq)
	_, oBody := c.do(http.MethodPost, "/api/v1/auth/signup", "", map[string]string{
		"household_name": "Other Household", "display_name": "Stranger",
		"email": otherEmail, "password": "strangerpass",
	}, "")
	otherTok := oBody["access_token"].(string)
	if _, listBody := c.do(http.MethodGet, "/api/v1/accounts", otherTok, nil, ""); listBody["accounts"] != nil {
		if accts, ok := listBody["accounts"].([]any); ok && len(accts) != 0 {
			t.Fatalf("cross-tenant leak: other household sees %d accounts", len(accts))
		}
	}
	if stX, _ := c.do(http.MethodGet, "/api/v1/accounts/"+acctID, otherTok, nil, ""); stX != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant account access, got %d", stX)
	}
	if stX, _ := c.do(http.MethodPost, "/api/v1/transactions/"+earnTxID+"/settle", otherTok, nil, idem("x-settle")); stX != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant settle, got %d", stX)
	}
}
