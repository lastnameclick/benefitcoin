// Command api is the BenefitCoins core-banking HTTP server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cpal/internal/api"
	"cpal/internal/auth"
	"cpal/internal/config"
	"cpal/internal/ledger"
	"cpal/internal/notify"
	"cpal/internal/store"
)

// bountySweepInterval is how often the background sweep checks for bounties
// expiring soon or already past their deadline.
const bountySweepInterval = time.Minute

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Postgres.
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	log.Println("postgres connected and migrated")

	// TigerBeetle.
	lg, err := ledger.Connect(cfg.TBClusterID, []string{cfg.TBAddress})
	if err != nil {
		return err
	}
	defer lg.Close()
	log.Println("tigerbeetle connected")

	am := auth.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	nf := notify.New(st, cfg.Push)
	if !cfg.Push.Configured() {
		log.Println("VAPID keys not configured — push notifications disabled, in-app feed still works")
	}
	srv := api.NewServer(cfg, st, lg, am, nf)

	// Periodic bounty sweep: warns holders about bounties expiring soon and
	// retires (with a notification) any that already expired. Runs in-process
	// since there's no separate job runner for anything this frequent.
	go func() {
		ticker := time.NewTicker(bountySweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nf.SweepBounties(ctx)
			}
		}
	}()

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutdownCtx)
		cancel()
	}()

	log.Printf("listening on %s", cfg.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
