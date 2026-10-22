// Command server runs the GoScrape API: it accepts crawl requests, runs them on
// a concurrent worker pool and stores the results in SQLite.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goscrape/backend/internal/crawler"
	"github.com/goscrape/backend/internal/parser"
	"github.com/goscrape/backend/internal/storage"
)

type config struct {
	addr         string
	dbPath       string
	workers      int
	maxPages     int
	rate         float64
	burst        int
	timeout      time.Duration
	allowPrivate bool
	corsOrigin   string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.addr, "addr", envOr("GOSCRAPE_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.dbPath, "db", envOr("GOSCRAPE_DB", "goscrape.db"), "SQLite database file")
	flag.IntVar(&cfg.workers, "workers", 8, "concurrent fetch workers per crawl")
	flag.IntVar(&cfg.maxPages, "max-pages", 500, "maximum pages fetched per crawl")
	flag.Float64Var(&cfg.rate, "rate", 2, "maximum requests per second per domain")
	flag.IntVar(&cfg.burst, "burst", 2, "token bucket burst size per domain")
	flag.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "timeout for each page request")
	flag.BoolVar(&cfg.allowPrivate, "allow-private", false, "allow crawling loopback/private addresses (disables SSRF protection)")
	flag.StringVar(&cfg.corsOrigin, "cors-origin", envOr("GOSCRAPE_CORS_ORIGIN", "http://localhost:3000"), "browser origin allowed to call the API directly (empty disables CORS)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := storage.Open(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	n, err := store.FailInterrupted(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted jobs: %w", err)
	}
	if n > 0 {
		log.Warn("marked interrupted jobs as failed", "count", n)
	}

	fetcher := parser.NewFetcher(parser.Options{
		Timeout:      cfg.timeout,
		AllowPrivate: cfg.allowPrivate,
	})
	limiter := crawler.NewDomainLimiter(cfg.rate, cfg.burst)
	manager := crawler.NewManager(ctx, store, fetcher, limiter,
		crawler.Config{Workers: cfg.workers, MaxPages: cfg.maxPages}, log)

	a := &api{store: store, crawls: manager, log: log}
	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           a.routes(cfg.corsOrigin),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.addr, "db", cfg.dbPath,
			"workers", cfg.workers, "rate_per_domain", cfg.rate, "allow_private", cfg.allowPrivate)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "err", err)
	}
	if err := manager.Shutdown(shutdownCtx); err != nil {
		log.Error("crawler shutdown", "err", err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
