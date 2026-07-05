package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadReadsDotEnv verifies the fix for VAPID (and every other) env var
// not being picked up from .env — Load() previously read only the real
// process environment, so a value pasted into .env had no effect unless the
// shell exported it first.
func TestLoadReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("VAPID_PUBLIC_KEY=from-dotenv-pub\nVAPID_PRIVATE_KEY=from-dotenv-priv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Push.PublicKey != "from-dotenv-pub" || cfg.Push.PrivateKey != "from-dotenv-priv" {
		t.Fatalf("Push = %+v, want values from .env", cfg.Push)
	}
	if !cfg.Push.Configured() {
		t.Fatal("Configured() = false, want true once both VAPID keys are set")
	}
}

// TestLoadPrefersRealEnvOverDotEnv verifies a real environment variable
// always wins over a conflicting .env value (dotenv's standard semantics),
// so container/CI deployments that set env vars directly aren't overridden.
func TestLoadPrefersRealEnvOverDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("VAPID_PUBLIC_KEY=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("VAPID_PUBLIC_KEY", "from-real-env")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Push.PublicKey != "from-real-env" {
		t.Fatalf("PublicKey = %q, want the real env var to win", cfg.Push.PublicKey)
	}
}
