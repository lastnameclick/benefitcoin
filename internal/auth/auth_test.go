package auth

import (
	"testing"
	"time"

	"cpal/internal/domain"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "s3cret") {
		t.Error("correct password rejected")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password accepted")
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	m := NewManager("test-secret", 15*time.Minute, time.Hour)
	in := Claims{IdentityID: "id-1", CustomerID: "cust-1", Username: "kid", Role: domain.RoleHolder}
	tok, exp, err := m.IssueAccess(in, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !exp.After(time.Now()) {
		t.Error("expiry not in the future")
	}
	out, err := m.ParseAccess(tok)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: %+v != %+v", out, in)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	m := NewManager("test-secret", time.Minute, time.Hour)
	tok, _, err := m.IssueAccess(Claims{IdentityID: "x", Role: domain.RoleOperator}, time.Now().Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ParseAccess(tok); err == nil {
		t.Error("expected expired token to be rejected")
	}
}

func TestWrongSecretRejected(t *testing.T) {
	m1 := NewManager("secret-a", time.Minute, time.Hour)
	m2 := NewManager("secret-b", time.Minute, time.Hour)
	tok, _, _ := m1.IssueAccess(Claims{IdentityID: "x", Role: domain.RoleOperator}, time.Now())
	if _, err := m2.ParseAccess(tok); err == nil {
		t.Error("expected token signed with different secret to be rejected")
	}
}

func TestRefreshTokenHashing(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashRefresh(raw) != hash {
		t.Error("hash mismatch for refresh token")
	}
	raw2, _, _ := NewRefreshToken()
	if raw2 == raw {
		t.Error("refresh tokens should be unique")
	}
}
