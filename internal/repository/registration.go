package repository

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/redis/go-redis/v9"
)

type Repository struct {
	cfg    *config.Config
	logger *logx.Logger
	db     *pgxpool.Pool
	redis  *redis.Client
}

func New(cfg *config.Config, logger *logx.Logger, db *pgxpool.Pool, redis *redis.Client) *Repository {
	return &Repository{
		cfg:    cfg,
		logger: logger,
		db:     db,
		redis:  redis,
	}
}

// DB returns the PostgreSQL connection pool.
func (r *Repository) DB() *pgxpool.Pool {
	return r.db
}

// Redis returns the optional Redis client, or nil when Redis is disabled.
func (r *Repository) Redis() *redis.Client {
	return r.redis
}
