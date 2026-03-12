package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"templates-registry/internal/domain"
)

type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository создает репозиторий пользователей на PostgreSQL.
func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create сохраняет нового пользователя в базе данных.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (
			id, username, password_hash, role, is_permanent_ban, banned_until, ban_duration_days, created_at
		)
		VALUES (
			:id, :username, :password_hash, :role, :is_permanent_ban, :banned_until, :ban_duration_days, :created_at
		)
	`
	_, err := sqlx.NamedExecContext(ctx, getExecutor(ctx, r.db), query, user)
	return err
}

// GetByUsername возвращает пользователя по имени.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `
		SELECT id, username, password_hash, role, is_permanent_ban, banned_until, ban_duration_days, created_at
		FROM users
		WHERE username = $1
	`

	var user domain.User
	err := getExecutor(ctx, r.db).GetContext(ctx, &user, query, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// List возвращает список всех пользователей.
func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	query := `
		SELECT id, username, password_hash, role, is_permanent_ban, banned_until, ban_duration_days, created_at
		FROM users
		ORDER BY username ASC
	`

	users := []domain.User{}
	if err := getExecutor(ctx, r.db).SelectContext(ctx, &users, query); err != nil {
		return nil, err
	}

	return users, nil
}

// UpdateRole меняет роль пользователя по имени.
func (r *UserRepository) UpdateRole(ctx context.Context, username, role string) error {
	query := `UPDATE users SET role = $1 WHERE username = $2`
	result, err := getExecutor(ctx, r.db).ExecContext(ctx, query, role, username)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// UpdateBan обновляет состояние блокировки пользователя.
func (r *UserRepository) UpdateBan(ctx context.Context, username string, permanent bool, bannedUntil *time.Time, durationDays *int) error {
	query := `
		UPDATE users
		SET is_permanent_ban = $1,
			banned_until = $2,
			ban_duration_days = $3
		WHERE username = $4
	`
	result, err := getExecutor(ctx, r.db).ExecContext(ctx, query, permanent, bannedUntil, durationDays, username)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// ClearBan снимает блокировку и очищает ее метаданные.
func (r *UserRepository) ClearBan(ctx context.Context, username string) error {
	query := `
		UPDATE users
		SET is_permanent_ban = FALSE,
			banned_until = NULL,
			ban_duration_days = NULL
		WHERE username = $1
	`
	result, err := getExecutor(ctx, r.db).ExecContext(ctx, query, username)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}
