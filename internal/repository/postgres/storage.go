package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"templates-registry/internal/domain"
)

type DBVersionRepository struct {
	db *sqlx.DB
}

// NewDBVersionRepository создает репозиторий версии базы данных.
func NewDBVersionRepository(db *sqlx.DB) *DBVersionRepository {
	return &DBVersionRepository{db: db}
}

// Ensure гарантирует существование записи о версии схемы.
func (r *DBVersionRepository) Ensure(ctx context.Context, version string) error {
	query := `
		INSERT INTO db_version (id, version, revision, updated_at)
		VALUES (1, $1, 1, NOW())
		ON CONFLICT (id) DO UPDATE
		SET version = EXCLUDED.version
	`
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, query, version)
	return err
}

// Get возвращает текущую версию схемы и глобальную ревизию.
func (r *DBVersionRepository) Get(ctx context.Context) (*domain.DBVersion, error) {
	query := `
		SELECT id, version, revision, updated_at
		FROM db_version
		WHERE id = 1
	`

	var item domain.DBVersion
	err := getExecutor(ctx, r.db).GetContext(ctx, &item, query)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &item, nil
}

// Increment увеличивает глобальную ревизию синхронизации.
func (r *DBVersionRepository) Increment(ctx context.Context) error {
	query := `
		UPDATE db_version
		SET revision = revision + 1,
			updated_at = NOW()
		WHERE id = 1
	`
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, query)
	return err
}
