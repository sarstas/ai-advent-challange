// Package store — доступ к PostgreSQL: миграции, запись и чтение истории запросов.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Request — одна строка журнала обращений к LLM.
type Request struct {
	ID               int64
	CreatedAt        time.Time
	Model            string
	Prompt           string
	Response         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMS        int
	Status           string // ok | error
	ErrorMessage     string
}

const columns = `id, created_at, model, prompt, response,
	prompt_tokens, completion_tokens, total_tokens,
	latency_ms, status, error_message`

// Store держит пул соединений с PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New подключается к БД и проверяет, что она отвечает.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("разбор DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 8
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("создание пула соединений: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL недоступен (проверь `make db-up`): %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close закрывает пул.
func (s *Store) Close() { s.pool.Close() }

// Ping проверяет живость соединения — используется в /api/health.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate применяет неприменённые миграции из migrations/*.sql.
// Каждый файл выполняется в своей транзакции и отмечается в schema_migrations.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("создание schema_migrations: %w", err)
	}

	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries) // 0001_, 0002_, ... — порядок применения

	for _, name := range entries {
		applied, err := s.isApplied(ctx, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("миграция %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("отметка миграции %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit миграции %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) isApplied(ctx context.Context, version string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
	return exists, err
}

// Save пишет обращение к LLM и возвращает строку с проставленными id и created_at.
// Вызывается и при успехе, и при ошибке — иначе не отладить ни 429, ни таймауты.
func (s *Store) Save(ctx context.Context, r Request) (Request, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO llm_requests
			(model, prompt, response, prompt_tokens, completion_tokens, total_tokens, latency_ms, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		r.Model, r.Prompt, r.Response, r.PromptTokens, r.CompletionTokens,
		r.TotalTokens, r.LatencyMS, r.Status, r.ErrorMessage,
	).Scan(&r.ID, &r.CreatedAt)
	if err != nil {
		return Request{}, fmt.Errorf("запись в llm_requests: %w", err)
	}
	return r, nil
}

// List возвращает последние limit записей (свежие сверху).
func (s *Store) List(ctx context.Context, limit int) ([]Request, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+columns+`
		FROM llm_requests
		ORDER BY created_at DESC, id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("чтение llm_requests: %w", err)
	}
	defer rows.Close()

	items := make([]Request, 0, limit)
	for rows.Next() {
		var r Request
		if err := rows.Scan(
			&r.ID, &r.CreatedAt, &r.Model, &r.Prompt, &r.Response,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens,
			&r.LatencyMS, &r.Status, &r.ErrorMessage,
		); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// Stats — маленькая сводка для шапки UI.
func (s *Store) Stats(ctx context.Context) (total int, errors int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = 'error')
		FROM llm_requests`).Scan(&total, &errors)
	return total, errors, err
}
