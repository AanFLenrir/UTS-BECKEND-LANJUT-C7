package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"latihan-fiber/UTS/config"
)

func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.Get("DB_USER", "postgres"),
		config.Get("DB_PASSWORD", ""),
		config.Get("DB_HOST", "localhost"),
		config.Get("DB_PORT", "5432"),
		config.Get("DB_NAME", "siakad_mini"),
		config.Get("DB_SSLMODE", "disable"),
	))
	if err != nil {
		return nil, fmt.Errorf("konfigurasi PostgreSQL tidak valid: %w", err)
	}
	cfg.MaxConns = int32(config.GetInt("DB_MAX_CONNS", 10))
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("membuat pool PostgreSQL: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return pool, nil
}
