// Command onerep is the onerep server and its maintenance subcommands.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/config"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web"
)

const usage = `usage: onerep [command]

commands:
  serve          run the web server (default)
  migrate        apply database migrations and exit
  backup <path>  write a consistent copy of the database to <path>
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	setupLogging(cfg)

	switch cmd {
	case "serve":
		return serve(ctx, cfg)
	case "migrate":
		return store.Migrate(cfg.DBPath)
	case "backup":
		if len(args) != 1 {
			return errors.New("backup: expected exactly one destination path")
		}
		db, err := store.Open(ctx, cfg.DBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Backup(ctx, args[0])
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func setupLogging(cfg config.Config) {
	var h slog.Handler
	if cfg.Dev() {
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		h = slog.NewJSONHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(h))
}

func serve(ctx context.Context, cfg config.Config) error {
	if cfg.AutoMigrate {
		if err := store.Migrate(cfg.DBPath); err != nil {
			return err
		}
	}
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	sessions := &auth.Sessions{Store: db, Secure: cfg.SecureCookies()}
	srv := &web.Server{DB: db, Sessions: sessions}
	if cfg.OIDC.Issuer != "" {
		srv.OIDC, err = auth.NewOIDC(ctx, cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, cfg.BaseURL, sessions)
		if err != nil {
			return err
		}
	}
	if cfg.DevUser != "" {
		slog.Warn("DEV LOGIN BYPASS ENABLED: every visitor is signed in as " + cfg.DevUser)
		srv.DevUser = cfg.DevUser
	}

	go cleanupSessions(ctx, db)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen, "base_url", cfg.BaseURL)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

// cleanupSessions deletes expired auth sessions once an hour.
func cleanupSessions(ctx context.Context, db *store.DB) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := db.DeleteExpiredAuthSessions(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("cleanup sessions", "err", err)
		} else if n > 0 {
			slog.Info("cleanup sessions", "deleted", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
