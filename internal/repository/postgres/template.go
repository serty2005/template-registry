package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
	"templates-registry/internal/domain"
)

type dbExecutor interface {
	sqlx.ExtContext
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

// getExecutor выбирает транзакцию из контекста или обычное подключение к базе.
func getExecutor(ctx context.Context, db *sqlx.DB) dbExecutor {
	if tx, ok := ctx.Value(txKey{}).(*sqlx.Tx); ok {
		return tx
	}
	return db
}

// EntityRepository реализует доступ к сущностям в PostgreSQL.
type EntityRepository struct {
	db *sqlx.DB
}

// NewEntityRepository создает репозиторий сущностей на PostgreSQL.
func NewEntityRepository(db *sqlx.DB) *EntityRepository {
	return &EntityRepository{db: db}
}

// List возвращает список сущностей с учетом заданных фильтров.
func (r *EntityRepository) List(ctx context.Context, filter domain.EntityListFilter) ([]domain.Entity, error) {
	query := `
		SELECT id, name, type, deleted, revision, created_by, created_at, updated_at
		FROM entities
	`
	args := []interface{}{}
	conditions := []string{}

	if filter.Type != "" {
		args = append(args, filter.Type)
		conditions = append(conditions, "type = ?")
	}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "deleted = FALSE")
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY name ASC, id ASC"

	query = sqlx.Rebind(sqlx.DOLLAR, query)

	entities := []domain.Entity{}
	if err := getExecutor(ctx, r.db).SelectContext(ctx, &entities, query, args...); err != nil {
		return nil, err
	}

	return entities, nil
}

// GetByID возвращает сущность по ее идентификатору.
func (r *EntityRepository) GetByID(ctx context.Context, entityID string) (*domain.Entity, error) {
	var entity domain.Entity
	query := `
		SELECT id, name, type, deleted, revision, created_by, created_at, updated_at
		FROM entities
		WHERE id = $1
	`

	err := getExecutor(ctx, r.db).GetContext(ctx, &entity, query, entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// Create сохраняет новую сущность в базе данных.
func (r *EntityRepository) Create(ctx context.Context, entity *domain.Entity) error {
	query := `
		INSERT INTO entities (id, name, type, deleted, revision, created_by, created_at, updated_at)
		VALUES (:id, :name, :type, :deleted, :revision, :created_by, :created_at, :updated_at)
	`
	_, err := sqlx.NamedExecContext(ctx, getExecutor(ctx, r.db), query, entity)
	return err
}

// Update обновляет сущность и возвращает новый номер ревизии.
func (r *EntityRepository) Update(ctx context.Context, entityID, name, entityType string) (int, error) {
	var revision int
	query := `
		UPDATE entities
		SET name = $1,
			type = $2,
			revision = revision + 1,
			updated_at = NOW()
		WHERE id = $3 AND deleted = FALSE
		RETURNING revision
	`

	err := getExecutor(ctx, r.db).GetContext(ctx, &revision, query, name, entityType, entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return revision, err
}

// SoftDelete выполняет мягкое удаление сущности и увеличивает ревизию.
func (r *EntityRepository) SoftDelete(ctx context.Context, entityID string) (int, error) {
	var revision int
	query := `
		UPDATE entities
		SET deleted = TRUE,
			revision = revision + 1,
			updated_at = NOW()
		WHERE id = $1
		RETURNING revision
	`

	err := getExecutor(ctx, r.db).GetContext(ctx, &revision, query, entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return revision, err
}
