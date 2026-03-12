package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

// Константы ролей пользователя в системе.
const (
	RoleUser      = "user"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// User описывает пользователя сервиса шаблонов.
type User struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	Username        string     `db:"username" json:"username"`
	PasswordHash    string     `db:"password_hash" json:"-"`
	Role            string     `db:"role" json:"role"`
	IsPermanentBan  bool       `db:"is_permanent_ban" json:"is_permanent_ban"`
	BannedUntil     *time.Time `db:"banned_until" json:"banned_until"`
	BanDurationDays *int       `db:"ban_duration_days" json:"ban_duration_days,omitempty"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
}

// DBVersion хранит версию схемы и глобальную ревизию синхронизации.
type DBVersion struct {
	ID             int       `db:"id" json:"-"`
	Version        string    `db:"version" json:"version"`
	GlobalRevision int       `db:"revision" json:"global_revision"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// Entity описывает логическую сущность шаблона в каталоге клиента.
type Entity struct {
	ID        string     `db:"id" json:"id"`
	Name      string     `db:"name" json:"name"`
	Type      string     `db:"type" json:"type"`
	Deleted   bool       `db:"deleted" json:"deleted"`
	Revision  int        `db:"revision" json:"revision"`
	CreatedBy *uuid.UUID `db:"created_by" json:"created_by,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
}

// EntityListFilter задает фильтры выборки сущностей.
type EntityListFilter struct {
	Type           string
	IncludeDeleted bool
}

// TemplateRevision хранит конкретную ревизию шаблона и визуализации.
type TemplateRevision struct {
	ID            string     `db:"id" json:"templateId"`
	EntityID      string     `db:"entity_id" json:"entity_id"`
	Revision      int        `db:"revision" json:"revision"`
	Template      string     `db:"template" json:"template"`
	Visualization string     `db:"visualization" json:"visualization"`
	Author        string     `db:"author_username" json:"author"`
	CreatedBy     *uuid.UUID `db:"created_by" json:"created_by,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

// RevisionListItem хранит метаданные ревизии для истории изменений.
type RevisionListItem struct {
	EntityID   string    `db:"entity_id" json:"entity_id"`
	Revision   int       `db:"revision" json:"revision"`
	TemplateID string    `db:"id" json:"templateId"`
	Author     string    `db:"author_username" json:"author"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}
