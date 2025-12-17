package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config describes how to connect to Postgres.
type Config struct {
	// DSN is the full Postgres connection string.
	// Examples:
	// - postgres://user:pass@localhost:5432/bes_blind?sslmode=disable
	// - postgresql://user:pass@db:5432/bes_blind?sslmode=disable
	DSN string

	// MaxConns limits the pool size. If 0, a sensible default is used.
	MaxConns int32

	// MinConns keeps warm connections. If 0, default is used.
	MinConns int32

	// ConnMaxLifetime closes/recreates conns after this duration. If 0, default is used.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime closes idle conns after this duration. If 0, default is used.
	ConnMaxIdleTime time.Duration

	// HealthCheckPeriod controls pool health checks. If 0, default is used.
	HealthCheckPeriod time.Duration

	// ConnectTimeout bounds the initial connect/ping. If 0, default is used.
	ConnectTimeout time.Duration
}

// DefaultConfigFromEnv returns a Config built from environment variables.
// Supported env vars:
// - DATABASE_URL (preferred) OR BES_DATABASE_URL
// - BES_DB_MAX_CONNS (optional int)
// - BES_DB_MIN_CONNS (optional int)
// - BES_DB_CONN_MAX_LIFETIME (optional duration, e.g. "30m")
// - BES_DB_CONN_MAX_IDLE_TIME (optional duration, e.g. "5m")
// - BES_DB_HEALTH_CHECK_PERIOD (optional duration, e.g. "30s")
// - BES_DB_CONNECT_TIMEOUT (optional duration, e.g. "5s")
func DefaultConfigFromEnv() (Config, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("BES_DATABASE_URL"))
	}
	if dsn == "" {
		return Config{}, errors.New("missing DATABASE_URL (or BES_DATABASE_URL)")
	}

	cfg := Config{
		DSN:               dsn,
		MaxConns:          parseInt32Env("BES_DB_MAX_CONNS", 10),
		MinConns:          parseInt32Env("BES_DB_MIN_CONNS", 0),
		ConnMaxLifetime:   parseDurationEnv("BES_DB_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime:   parseDurationEnv("BES_DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		HealthCheckPeriod: parseDurationEnv("BES_DB_HEALTH_CHECK_PERIOD", 30*time.Second),
		ConnectTimeout:    parseDurationEnv("BES_DB_CONNECT_TIMEOUT", 5*time.Second),
	}

	return cfg, nil
}

// Open creates a pgxpool.Pool and verifies connectivity with Ping.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("empty DSN")
	}

	normalized, err := normalizePostgresURL(cfg.DSN)
	if err != nil {
		return nil, err
	}

	poolCfg, err := pgxpool.ParseConfig(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	// Apply tuning.
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	}
	if cfg.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	pingTimeout := cfg.ConnectTimeout
	if pingTimeout <= 0 {
		pingTimeout = 5 * time.Second
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

// normalizePostgresURL ensures sslmode is set (defaults to "disable" if absent)
// and accepts both postgres:// and postgresql:// schemes.
func normalizePostgresURL(dsn string) (string, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", errors.New("empty postgres url")
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse postgres url: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
		// ok
	default:
		// If it's not a URL, pgx can still parse keyword DSN (user=...),
		// so just return as-is.
		return dsn, nil
	}

	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

func parseInt32Env(key string, def int32) int32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	// avoid strconv import to keep this small; do minimal parse
	var n int64
	sign := int64(1)
	i := 0
	if len(v) > 0 && v[0] == '-' {
		sign = -1
		i = 1
	}
	for ; i < len(v); i++ {
		c := v[i]
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int64(c-'0')
		if n > 1<<31-1 {
			return def
		}
	}
	n *= sign
	if n <= 0 {
		return def
	}
	return int32(n)
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
