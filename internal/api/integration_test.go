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

// doRaw is like do but returns the undecoded response — needed for binary
// (PDF) responses that aren't JSON.
func (c *apiClient) doRaw(method, path, token string) (int, http.Header, []byte) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, nil)
	if err != nil {
		c.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, raw
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

// mustBalance is a terser balanceOf for tests that only care about current and awaiting.
func mustBalance(t *testing.T, c *apiClient, token, acctID string) (current, awaiting int64) {
	t.Helper()
	cur, _, await := balanceOf(c, token, acctID)
	return cur, await
}

// taskListHas reports whether the given task id appears in the caller's task list.
func taskListHas(c *apiClient, token, taskID string) bool {
	_, body := c.do(http.MethodGet, "/api/v1/tasks", token, nil, "")
	tasks, _ := body["tasks"].([]any)
	for _, raw := range tasks {
		if m, ok := raw.(map[string]any); ok && m["id"] == taskID {
			return true
		}
	}
	return false
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

	// Charts: the kid's own balance-history, earn/redeem, and leaderboard
	// endpoints all resolve against the settled activity above.
	if stC, cbody := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/charts/balance-history", kidTok, nil, ""); stC != 200 {
		t.Fatalf("balance-history: %d %v", stC, cbody)
	} else if pts, ok := cbody["points"].([]any); !ok || len(pts) == 0 {
		t.Fatalf("balance-history: expected at least one point, got %v", cbody["points"])
	}
	if stC, cbody := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/charts/earn-redeem", kidTok, nil, ""); stC != 200 {
		t.Fatalf("earn-redeem: %d %v", stC, cbody)
	}
	if stC, cbody := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/charts/redemption-frequency", kidTok, nil, ""); stC != 200 {
		t.Fatalf("redemption-frequency: %d %v", stC, cbody)
	} else if cbody["by_hour"] == nil || cbody["by_weekday"] == nil || cbody["by_month"] == nil {
		t.Fatalf("redemption-frequency: missing breakdowns: %v", cbody)
	}
	if stC, cbody := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/charts/task-leaderboard", kidTok, nil, ""); stC != 200 {
		t.Fatalf("task-leaderboard: %d %v", stC, cbody)
	} else if entries, ok := cbody["entries"].([]any); !ok || len(entries) == 0 {
		t.Fatalf("task-leaderboard: expected earned tasks to appear, got %v", cbody["entries"])
	}

	// Operator-only tenant-wide views: a holder is forbidden, the operator sees data.
	if stF, _ := c.do(http.MethodGet, "/api/v1/tenant/charts/household-overview", kidTok, nil, ""); stF != http.StatusForbidden {
		t.Fatalf("expected 403 for holder household-overview, got %d", stF)
	}
	if stH, hbody := c.do(http.MethodGet, "/api/v1/tenant/charts/household-overview", opTok, nil, ""); stH != 200 {
		t.Fatalf("household-overview: %d %v", stH, hbody)
	} else if holders, ok := hbody["holders"].([]any); !ok || len(holders) != 1 {
		t.Fatalf("household-overview: expected 1 holder, got %v", hbody["holders"])
	}

	// Statement PDF: on-demand download returns a real PDF but is a one-off
	// preview/reprint — it must NOT be saved to the Inbox (repeatedly clicking
	// "generate" for the same month shouldn't pile up duplicate Inbox entries).
	stP, hdr, pdfBytes := c.doRaw(http.MethodGet, "/api/v1/accounts/"+acctID+"/statement.pdf", kidTok)
	if stP != 200 {
		t.Fatalf("statement.pdf: %d", stP)
	}
	if ct := hdr.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("statement.pdf: expected application/pdf, got %q", ct)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatalf("statement.pdf: response does not look like a PDF (first bytes: %q)", pdfBytes[:min(16, len(pdfBytes))])
	}
	stI0, ibody0 := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/inbox", kidTok, nil, "")
	if stI0 != 200 {
		t.Fatalf("inbox: %d %v", stI0, ibody0)
	}
	if statements, _ := ibody0["statements"].([]any); len(statements) != 0 {
		t.Fatalf("inbox: on-demand download must not be saved, got %v", statements)
	}

	// Inbox is populated only by the monthly job (simulated here via a direct
	// store write, since that binary isn't exercised by this HTTP test).
	// Listed and re-downloadable statements should reflect that seeded row.
	tenantID := suBody["tenant_id"].(string)
	if _, err := st.SaveStatement(context.Background(), tenantID, acctID, time.Now().UTC().AddDate(0, -1, 0), pdfBytes); err != nil {
		t.Fatalf("seed statement: %v", err)
	}
	stI, ibody := c.do(http.MethodGet, "/api/v1/accounts/"+acctID+"/inbox", kidTok, nil, "")
	if stI != 200 {
		t.Fatalf("inbox: %d %v", stI, ibody)
	}
	statements, _ := ibody["statements"].([]any)
	if len(statements) != 1 {
		t.Fatalf("inbox: expected 1 statement, got %v", statements)
	}
	stmtID := statements[0].(map[string]any)["id"].(string)
	stD, dhdr, dpdf := c.doRaw(http.MethodGet, "/api/v1/accounts/"+acctID+"/inbox/"+stmtID+"/pdf", kidTok)
	if stD != 200 || dhdr.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(dpdf, []byte("%PDF")) {
		t.Fatalf("inbox download: status=%d content-type=%q", stD, dhdr.Get("Content-Type"))
	}

	// --- Chore proposals: a kid can request coins for something not on the catalog. ---
	_, awaitBefore := mustBalance(t, c, opTok, acctID)

	stProp, propBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings/custom", kidTok, map[string]string{
		"description": "Organized the garage", "value": "0.20",
	}, idem("propose-1"))
	if stProp != 201 {
		t.Fatalf("propose chore: %d %v", stProp, propBody)
	}
	proposalID := propBody["id"].(string)
	if propBody["task_id"] != nil {
		t.Fatalf("a proposed chore should have no task_id, got %v", propBody["task_id"])
	}
	if _, await := mustBalance(t, c, opTok, acctID); await != awaitBefore+200 {
		t.Fatalf("after propose want awaiting +200 from %d, got %d", awaitBefore, await)
	}

	// Only operators may adjust the proposed reward.
	if stF, _ := c.do(http.MethodPost, "/api/v1/transactions/"+proposalID+"/adjust", kidTok, map[string]string{"amount": "0.50"}, idem("kid-adjust")); stF != http.StatusForbidden {
		t.Fatalf("expected 403 for holder adjust, got %d", stF)
	}

	// Operator revises the reward up before approving.
	if stAdj, adjBody := c.do(http.MethodPost, "/api/v1/transactions/"+proposalID+"/adjust", opTok, map[string]string{"amount": "0.50"}, idem("adjust-1")); stAdj != 200 {
		t.Fatalf("adjust: %d %v", stAdj, adjBody)
	}
	if _, await := mustBalance(t, c, opTok, acctID); await != awaitBefore+500 {
		t.Fatalf("after adjust want awaiting +500 from %d, got %d", awaitBefore, await)
	}

	curBeforeSettle, _ := mustBalance(t, c, opTok, acctID)
	if stS, sBody := c.do(http.MethodPost, "/api/v1/transactions/"+proposalID+"/settle", opTok, nil, idem("settle-proposal")); stS != 200 {
		t.Fatalf("settle proposal: %d %v", stS, sBody)
	}
	if cur, await := mustBalance(t, c, opTok, acctID); cur != curBeforeSettle+500 || await != awaitBefore {
		t.Fatalf("after settling the adjusted proposal want current +500 and awaiting back to %d, got cur=%d await=%d", awaitBefore, cur, await)
	}

	// --- Bounties: a one-time opportunity every holder sees, first to claim wins. ---
	_, bBody := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]any{
		"name": "Wash the car", "value": "0.75", "is_bounty": true,
	}, idem("bounty-1"))
	bountyID, _ := bBody["id"].(string)
	if bBody["is_bounty"] != true {
		t.Fatalf("expected is_bounty=true, got %v", bBody["is_bounty"])
	}

	// Onboard a second holder to race for the bounty.
	kid2User := fmt.Sprintf("kid2-%d", uniq)
	_, body2 := c.do(http.MethodPost, "/api/v1/customers", opTok, map[string]string{
		"type": "holder", "display_name": "Kiddo2", "username": kid2User, "password": "kidpass2",
	}, idem("cust2"))
	acct2 := body2["account"].(map[string]any)
	acct2ID := acct2["id"].(string)
	kid2Tok := login(c, kid2User, "kidpass2")

	if !taskListHas(c, kid2Tok, bountyID) {
		t.Fatal("bounty should be visible to holders before it's claimed")
	}

	// First kid claims it.
	stClaim, claimBody := c.do(http.MethodPost, "/api/v1/accounts/"+acctID+"/earnings", kidTok, map[string]string{"task_id": bountyID}, idem("claim-1"))
	if stClaim != 201 {
		t.Fatalf("claim bounty: %d %v", stClaim, claimBody)
	}
	claimTxID := claimBody["id"].(string)

	// Second kid is too late — it's already claimed.
	if stLate, lateBody := c.do(http.MethodPost, "/api/v1/accounts/"+acct2ID+"/earnings", kid2Tok, map[string]string{"task_id": bountyID}, idem("claim-2")); stLate != http.StatusConflict {
		t.Fatalf("expected 409 bounty_claimed for second claim, got %d %v", stLate, lateBody)
	}
	if taskListHas(c, kid2Tok, bountyID) {
		t.Fatal("claimed bounty should be hidden from other holders")
	}

	// Declining the claim reopens the bounty for anyone.
	if stV, vBody := c.do(http.MethodPost, "/api/v1/transactions/"+claimTxID+"/void", opTok, nil, idem("void-claim")); stV != 200 {
		t.Fatalf("void claim: %d %v", stV, vBody)
	}
	if !taskListHas(c, kid2Tok, bountyID) {
		t.Fatal("bounty should reopen after its claim is declined")
	}
	if stClaim2, claim2Body := c.do(http.MethodPost, "/api/v1/accounts/"+acct2ID+"/earnings", kid2Tok, map[string]string{"task_id": bountyID}, idem("claim-3")); stClaim2 != 201 {
		t.Fatalf("reclaim bounty: %d %v", stClaim2, claim2Body)
	}

	// --- Bounties can be timeboxed: expires_at is rejected unless it's a future date on a bounty. ---
	if stBad, badBody := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]any{
		"name": "Not a bounty", "value": "0.10", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
	}, idem("bad-expiry-1")); stBad != http.StatusBadRequest {
		t.Fatalf("expected 400 for expires_at on a non-bounty task, got %d %v", stBad, badBody)
	}
	if stBad, badBody := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]any{
		"name": "Already over", "value": "0.10", "is_bounty": true,
		"expires_at": time.Now().Add(-time.Hour).Format(time.RFC3339),
	}, idem("bad-expiry-2")); stBad != http.StatusBadRequest {
		t.Fatalf("expected 400 for a past expires_at, got %d %v", stBad, badBody)
	}

	// A bounty with a near deadline disappears and becomes unclaimable once it passes.
	_, expBody := c.do(http.MethodPost, "/api/v1/tasks", opTok, map[string]any{
		"name": "Blink and it's gone", "value": "0.30", "is_bounty": true,
		"expires_at": time.Now().Add(1500 * time.Millisecond).Format(time.RFC3339),
	}, idem("bounty-expiring"))
	expiringID, _ := expBody["id"].(string)
	if !taskListHas(c, kid2Tok, expiringID) {
		t.Fatal("a bounty should be visible before its deadline")
	}
	time.Sleep(2 * time.Second)
	if taskListHas(c, kid2Tok, expiringID) {
		t.Fatal("an expired bounty should be hidden even though nobody claimed it")
	}
	if stExp, expClaimBody := c.do(http.MethodPost, "/api/v1/accounts/"+acct2ID+"/earnings", kid2Tok, map[string]string{"task_id": expiringID}, idem("claim-expired")); stExp != http.StatusConflict {
		t.Fatalf("expected 409 bounty_expired for a claim after the deadline, got %d %v", stExp, expClaimBody)
	} else if expClaimBody["error"].(map[string]any)["code"] != "bounty_expired" {
		t.Fatalf("expected error code bounty_expired, got %v", expClaimBody["error"])
	}

	// It's also auto-retired (not just hidden), so the operator's catalog
	// reflects reality without a manual "Retire" click.
	_, opListBody := c.do(http.MethodGet, "/api/v1/tasks", opTok, nil, "")
	var expiredTask map[string]any
	for _, raw := range opListBody["tasks"].([]any) {
		if m := raw.(map[string]any); m["id"] == expiringID {
			expiredTask = m
		}
	}
	if expiredTask == nil {
		t.Fatal("operator should still see the expired bounty in their full task list")
	}
	if expiredTask["active"] != false {
		t.Fatalf("expired bounty should have been auto-retired (active=false), got %v", expiredTask["active"])
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
