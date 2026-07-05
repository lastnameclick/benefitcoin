// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime settings for the API server.
type Config struct {
	Addr        string // HTTP listen address, e.g. ":8080"
	DatabaseURL string // Postgres connection string
	TBAddress   string // TigerBeetle replica address(es), comma-separated
	TBClusterID uint64 // TigerBeetle cluster id (0 for local dev)

	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	CORSOrigin string // allowed SPA origin for browser requests

	// White-label branding — the SaaS operator sets these per deployment. They're
	// display-only (the internal ledger currency stays "BNC"); the SPA fetches
	// them from GET /api/v1/config and renders them everywhere.
	Branding Branding

	// SMTP is optional: emailing monthly statements is a best-effort enhancement
	// on top of the in-app Inbox, which always works with zero mail setup. An
	// empty Host means "not configured" — the statement job just skips sending.
	SMTP SMTPConfig

	// Push is optional: Web Push is a best-effort enhancement on top of the
	// in-app notification feed, which always works with zero VAPID setup. An
	// empty PublicKey means "not configured" — subscriptions are simply never
	// sent to.
	Push PushConfig
}

// PushConfig holds VAPID credentials for Web Push notifications.
type PushConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string // "mailto:" address or URL identifying the sender, per the VAPID spec
}

// Configured reports whether Web Push has been set up for this deployment.
func (c PushConfig) Configured() bool { return c.PublicKey != "" && c.PrivateKey != "" }

// SMTPConfig holds outbound mail settings for monthly statement emails.
type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
}

// Configured reports whether SMTP has been set up for this deployment.
func (c SMTPConfig) Configured() bool { return c.Host != "" }

// Branding is the operator-configurable naming shown throughout the UI.
type Branding struct {
	ProductName    string `json:"product_name"`     // in-app name / wordmark, e.g. "BenefitCoins"
	SiteName       string `json:"site_name"`        // marketing site / browser title
	CoinName       string `json:"coin_name"`        // singular unit, e.g. "coin"
	CoinNamePlural string `json:"coin_name_plural"` // plural unit, e.g. "coins"
	CoinCode       string `json:"coin_code"`        // short ticker shown next to amounts, e.g. "BNC"
}

// Load reads configuration from the environment, applying dev-friendly
// defaults. It first loads a .env file from the working directory if one
// exists — convenient for local dev (`make api` et al. run from the repo
// root) — without overriding variables already set in the real environment
// (e.g. in a container or CI). A missing .env file is not an error.
func Load() (Config, error) {
	_ = godotenv.Load()

	c := Config{
		Addr:        env("ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://cpal:cpal@localhost:5432/cpal?sslmode=disable"),
		TBAddress:   env("TB_ADDRESS", "3000"),
		JWTSecret:   env("JWT_SECRET", "dev-insecure-secret-change-me"),
		CORSOrigin:  env("CORS_ORIGIN", "http://localhost:5173"),
		Branding: Branding{
			ProductName:    env("PRODUCT_NAME", "BenefitCoins"),
			SiteName:       env("SITE_NAME", "BenefitCoins"),
			CoinName:       env("COIN_NAME", "coin"),
			CoinNamePlural: env("COIN_NAME_PLURAL", "coins"),
			CoinCode:       env("COIN_CODE", "BNC"),
		},
	}

	clusterID, err := strconv.ParseUint(env("TB_CLUSTER_ID", "0"), 10, 64)
	if err != nil {
		return c, fmt.Errorf("TB_CLUSTER_ID: %w", err)
	}
	c.TBClusterID = clusterID

	c.AccessTTL, err = durEnv("ACCESS_TTL", 15*time.Minute)
	if err != nil {
		return c, err
	}
	c.RefreshTTL, err = durEnv("REFRESH_TTL", 720*time.Hour) // 30 days
	if err != nil {
		return c, err
	}

	c.SMTP = SMTPConfig{
		Host:        env("SMTP_HOST", ""),
		Username:    env("SMTP_USERNAME", ""),
		Password:    env("SMTP_PASSWORD", ""),
		FromAddress: env("SMTP_FROM", "statements@localhost"),
	}
	if c.SMTP.Host != "" {
		port, err := strconv.Atoi(env("SMTP_PORT", "587"))
		if err != nil {
			return c, fmt.Errorf("SMTP_PORT: %w", err)
		}
		c.SMTP.Port = port
	}

	c.Push = PushConfig{
		PublicKey:  env("VAPID_PUBLIC_KEY", ""),
		PrivateKey: env("VAPID_PRIVATE_KEY", ""),
		Subject:    env("VAPID_SUBJECT", "mailto:admin@localhost"),
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}
