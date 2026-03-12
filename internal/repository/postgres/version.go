package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"templates-registry/internal/domain"
)

type TemplateRevisionRepository struct {
	db *sqlx.DB
}

// NewTemplateRevisionRepository создает репозиторий ревизий шаблонов.
func NewTemplateRevisionRepository(db *sqlx.DB) *TemplateRevisionRepository {
	return &TemplateRevisionRepository{db: db}
}

// Create сохраняет новую ревизию шаблона.
func (r *TemplateRevisionRepository) Create(ctx context.Context, revision *domain.TemplateRevision) error {
	query := `
		INSERT INTO template_revisions (
			id, entity_id, revision, template, visualization, author_username, created_by, created_at
		)
		VALUES (
			:id, :entity_id, :revision, :template, :visualization, :author_username, :created_by, :created_at
		)
	`
	_, err := sqlx.NamedExecContext(ctx, getExecutor(ctx, r.db), query, revision)
	return err
}

// ListByEntity возвращает все ревизии шаблона по сущности.
func (r *TemplateRevisionRepository) ListByEntity(ctx context.Context, entityID string) ([]domain.TemplateRevision, error) {
	query := `
		SELECT id, entity_id, revision, template, visualization, author_username, created_by, created_at
		FROM template_revisions
		WHERE entity_id = $1
		ORDER BY revision DESC
	`

	items := []domain.TemplateRevision{}
	if err := getExecutor(ctx, r.db).SelectContext(ctx, &items, query, entityID); err != nil {
		return nil, err
	}

	return items, nil
}

// ListRevisionMetaByEntity возвращает метаданные ревизий без тела шаблона.
func (r *TemplateRevisionRepository) ListRevisionMetaByEntity(ctx context.Context, entityID string) ([]domain.RevisionListItem, error) {
	query := `
		SELECT id, entity_id, revision, author_username, created_at
		FROM template_revisions
		WHERE entity_id = $1
		ORDER BY revision DESC
	`

	items := []domain.RevisionListItem{}
	if err := getExecutor(ctx, r.db).SelectContext(ctx, &items, query, entityID); err != nil {
		return nil, err
	}

	return items, nil
}

// GetByEntityRevision возвращает конкретную ревизию шаблона.
func (r *TemplateRevisionRepository) GetByEntityRevision(ctx context.Context, entityID string, revision int) (*domain.TemplateRevision, error) {
	query := `
		SELECT id, entity_id, revision, template, visualization, author_username, created_by, created_at
		FROM template_revisions
		WHERE entity_id = $1 AND revision = $2
	`

	var item domain.TemplateRevision
	err := getExecutor(ctx, r.db).GetContext(ctx, &item, query, entityID, revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &item, nil
}

// GetLatestByEntityIDs возвращает последние активные ревизии по списку сущностей.
func (r *TemplateRevisionRepository) GetLatestByEntityIDs(ctx context.Context, entityIDs []string) ([]domain.TemplateRevision, error) {
	if len(entityIDs) == 0 {
		return []domain.TemplateRevision{}, nil
	}

	query := `
		SELECT tr.id, tr.entity_id, tr.revision, tr.template, tr.visualization, tr.author_username, tr.created_by, tr.created_at
		FROM template_revisions tr
		INNER JOIN entities e
			ON e.id = tr.entity_id
			AND e.revision = tr.revision
		WHERE tr.entity_id = ANY($1)
			AND e.deleted = FALSE
		ORDER BY tr.entity_id ASC
	`

	items := []domain.TemplateRevision{}
	if err := getExecutor(ctx, r.db).SelectContext(ctx, &items, query, pq.Array(entityIDs)); err != nil {
		return nil, err
	}

	return items, nil
}
