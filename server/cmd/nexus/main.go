// Command nexus is the single native host process. It runs golang-migrate
// migrations, opens the Postgres pool, starts the background metric sampler,
// and serves the embedded SPA plus the JSON API from one chi router.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nexus/internal/auth"
	"nexus/internal/checker"
	"nexus/internal/config"
	"nexus/internal/db"
	"nexus/internal/host"
	"nexus/internal/httpapi"
	"nexus/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	log.Println("running migrations…")
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	authn := auth.New(cfg.PasswordHash, cfg.JWTSigningKey,
		auth.WithCookieSecure(cfg.CookieSecure),
		auth.WithTrustedProxy(cfg.TrustedProxy),
		auth.WithLimitPolicy(auth.LimitPolicy{
			MaxFailures: cfg.LoginMaxFailures,
			Window:      cfg.LoginWindow,
			Lockout:     cfg.LoginLockout,
		}),
	)
	realHost := host.NewRealHost()
	srv := httpapi.NewServer(pool, authn, realHost, checker.NewHTTPProber(), checker.NewHTTPFetcher())

	// Start the background sampler.
	go srv.Sampler().Run(ctx)

	// Start the background bookmark health checker.
	go srv.Checker().Run(ctx)

	handler := srv.Router(web.Handler())

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("nexus listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
