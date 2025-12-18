package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/valentin/bes-games/backend/internal/core"
	"github.com/valentin/bes-games/backend/internal/db"
	"github.com/valentin/bes-games/backend/internal/games/namethattune"
	"github.com/valentin/bes-games/backend/internal/httpapi"
	"github.com/valentin/bes-games/backend/internal/realtime"
)

const (
	defaultAddr         = ":8080"
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 10 * time.Second
	defaultIdleTimeout  = 60 * time.Second

	defaultMigrationsDir = "backend/migrations"
)

func main() {
	logger := log.New(os.Stdout, "api: ", log.LstdFlags|log.LUTC|log.Lmsgprefix)

	addr := envOrDefault("BES_ADDR", defaultAddr)

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Database ---
	cfg, err := db.DefaultConfigFromEnv()
	if err != nil {
		logger.Printf("missing DATABASE_URL (or BES_DATABASE_URL): %v", err)
		os.Exit(1)
	}

	pool, err := db.Open(ctx, cfg)
	if err != nil {
		logger.Printf("db open failed: %v", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Printf("connected to postgres")

	if err := runMigrations(ctx, logger, pool); err != nil {
		logger.Printf("migrations failed: %v", err)
		os.Exit(1)
	}

	// --- Realtime (in-memory fanout, DB remains source of truth) ---
	rt := realtime.NewRegistry()

	// --- Repo + API ---
	coreRepo := core.NewRepo(pool)
	nttRepo := namethattune.NewRepo(pool)
	api := httpapi.NewServer(coreRepo, nttRepo, rt)

	allowedOrigins := splitCommaEnv("BES_CORS_ALLOWED_ORIGINS")
	handler := api.Handler(httpapi.Options{
		AllowedOrigins: allowedOrigins,
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           withLogging(logger, withRecover(logger, handler)),
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Printf("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			logger.Printf("server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}

	logger.Printf("stopped")
}

func runMigrations(ctx context.Context, logger *log.Logger, pool *pgxpool.Pool) error {
	if os.Getenv("BES_MIGRATIONS_DISABLE") == "1" {
		logger.Printf("migrations disabled via BES_MIGRATIONS_DISABLE=1")
		return nil
	}

	dir := envOrDefault("BES_MIGRATIONS_DIR", defaultMigrationsDir)
	if !filepath.IsAbs(dir) {
		dir = filepath.Clean(dir)
	}

	// Goose requires *sql.DB. Use the pgx stdlib adapter.
	dbStd := stdlib.OpenDBFromPool(pool)
	defer func() { _ = dbStd.Close() }()

	goose.SetBaseFS(nil)

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	logger.Printf("running migrations from %s", dir)
	if err := goose.UpContext(ctx, dbStd, dir); err != nil {
		return err
	}

	logger.Printf("migrations up-to-date")
	return nil
}

func envOrDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func splitCommaEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func withLogging(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(lrw, r)

		logger.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, lrw.status, time.Since(start).String())
	})
}

func withRecover(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Printf("panic: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Hijack forwards http.Hijacker to the underlying ResponseWriter.
// This is required for WebSocket upgrades to work through middleware that wraps the writer.
func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hj.Hijack()
}

// Flush forwards http.Flusher when supported by the underlying ResponseWriter.
func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// compile-time assertion: ensure we imported database/sql (used by goose via pgx stdlib adapter)
var _ *sql.DB
