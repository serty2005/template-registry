package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"templates-registry/internal/config"
	"templates-registry/internal/domain"
	"templates-registry/internal/httpapi"
	"templates-registry/internal/pkg/logging"
	"templates-registry/internal/pkg/postgres"
	repopg "templates-registry/internal/repository/postgres"
)

// main поднимает конфиг, базу данных, репозитории и HTTP-сервер.
func main() {
	ctx := context.Background()
	jsonLogger := logging.NewJSONLogger()
	httpLogger := logging.NewJSONStdLogger()
	slog.SetDefault(jsonLogger)

	cfg, err := config.Load()
	if err != nil {
		logFatal(jsonLogger, "Ошибка загрузки конфигурации", err)
	}

	db, err := postgres.NewClient(ctx, cfg.Postgres)
	if err != nil {
		logFatal(jsonLogger, "Ошибка инициализации PostgreSQL", err)
	}
	defer db.Close()

	if err := applySchema(db); err != nil {
		logFatal(jsonLogger, "Ошибка применения схемы базы данных", err)
	}

	userRepo := repopg.NewUserRepository(db)
	versionRepo := repopg.NewDBVersionRepository(db)
	entityRepo := repopg.NewEntityRepository(db)
	revisionRepo := repopg.NewTemplateRevisionRepository(db)
	txManager := repopg.NewTransactionManager(db)

	if err := versionRepo.Ensure(ctx, cfg.AppVersion); err != nil {
		logFatal(jsonLogger, "Ошибка фиксации версии приложения в базе данных", err)
	}

	if err := ensureAdmin(ctx, userRepo, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		logFatal(jsonLogger, "Ошибка инициализации администратора", err)
	}

	handler := httpapi.NewServer(
		userRepo,
		versionRepo,
		entityRepo,
		revisionRepo,
		txManager,
		httpapi.Config{
			JWTSecret: cfg.JWTSecret,
			TokenTTL:  cfg.TokenTTL,
			Logger:    httpLogger,
		},
	)

	jsonLogger.Info("Сервер шаблонов запущен", slog.String("addr", cfg.AppAddr))
	if err := http.ListenAndServe(cfg.AppAddr, handler); err != nil {
		logFatal(jsonLogger, "Ошибка HTTP-сервера", err)
	}
}

// applySchema применяет SQL-схему из каталога миграций.
func applySchema(db *sqlx.DB) error {
	schemaPath, err := config.FindProjectFile("migrations/001_init_schema.sql")
	if err != nil {
		return err
	}

	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(sqlBytes))
	return err
}

// ensureAdmin создает администратора по умолчанию, если его еще нет.
func ensureAdmin(ctx context.Context, users domain.UserRepository, username, password string) error {
	if _, err := users.GetByUsername(ctx, username); err == nil {
		return nil
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin := &domain.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         domain.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}

	return users.Create(ctx, admin)
}

// logFatal пишет критическую ошибку в JSON-лог и завершает процесс.
func logFatal(logger *slog.Logger, message string, err error) {
	logger.Error(message, slog.Any("error", err))
	os.Exit(1)
}
