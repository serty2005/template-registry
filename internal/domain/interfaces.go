package domain

import (
	"context"
	"time"
)

// UserRepository управляет пользователями и их административным состоянием.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context) ([]User, error)
	UpdateRole(ctx context.Context, username, role string) error
	UpdateBan(ctx context.Context, username string, permanent bool, bannedUntil *time.Time, durationDays *int) error
	ClearBan(ctx context.Context, username string) error
}

// DBVersionRepository управляет глобальной ревизией и версией схемы.
type DBVersionRepository interface {
	Ensure(ctx context.Context, version string) error
	Get(ctx context.Context) (*DBVersion, error)
	Increment(ctx context.Context) error
}

// EntityRepository управляет сущностями шаблонов.
type EntityRepository interface {
	List(ctx context.Context, filter EntityListFilter) ([]Entity, error)
	GetByID(ctx context.Context, entityID string) (*Entity, error)
	Create(ctx context.Context, entity *Entity) error
	Update(ctx context.Context, entityID, name, entityType string) (int, error)
	SoftDelete(ctx context.Context, entityID string) (int, error)
}

// TemplateRevisionRepository управляет ревизиями шаблонов.
type TemplateRevisionRepository interface {
	Create(ctx context.Context, revision *TemplateRevision) error
	ListByEntity(ctx context.Context, entityID string) ([]TemplateRevision, error)
	ListRevisionMetaByEntity(ctx context.Context, entityID string) ([]RevisionListItem, error)
	GetByEntityRevision(ctx context.Context, entityID string, revision int) (*TemplateRevision, error)
	GetLatestByEntityIDs(ctx context.Context, entityIDs []string) ([]TemplateRevision, error)
}

// TransactionManager предоставляет выполнение операций в транзакции.
type TransactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
