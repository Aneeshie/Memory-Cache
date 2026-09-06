package repository

import (
	"context"
	"errors"

	"github.com/Aneeshie/shared-in-memory-index/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

var ErrNoRowRound = errors.New("No row found")

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) CreateCachedEntry(ctx context.Context, entry *domain.CachedEntry) error {
	query := `INSERT INTO cache_entries (key, data, created_at, updated_at)
	VALUES($1,$2,$3);
	`

	if _, err := r.db.Exec(ctx, query, entry.Data, entry.Key, entry.CreatedAt, entry.UpdatedAt); err != nil {
		return err
	}

	return nil
}

func (r *Repository) GetCachedEntry(ctx context.Context, key string) (*domain.CachedEntry, error) {
	var entry domain.CachedEntry

	query := `SELECT key, data, created_at, updated_at
		FROM cache_entries
		WHERE key = $1;
	`
	if err := r.db.QueryRow(ctx, query, key).Scan(&entry.Key, &entry.Data, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRowRound
		}
		return nil, err
	}

	return &entry, nil

}
