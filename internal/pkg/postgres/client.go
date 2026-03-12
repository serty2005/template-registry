package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Config описывает параметры подключения к PostgreSQL.
type Config struct {
	Host          string
	Port          string
	User          string
	Password      string
	DBName        string
	SSLMode       string
	MaintenanceDB string
}

// NewClient создает подключение к PostgreSQL и при необходимости поднимает базу данных.
func NewClient(ctx context.Context, cfg Config) (*sqlx.DB, error) {
	if err := ensureDatabase(ctx, cfg); err != nil {
		return nil, err
	}

	db, err := sqlx.ConnectContext(ctx, "postgres", cfg.dsn(cfg.DBName))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(2 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return db, nil
}

// ensureDatabase проверяет существование целевой базы и создает ее при отсутствии.
func ensureDatabase(ctx context.Context, cfg Config) error {
	if cfg.DBName == "" {
		return errors.New("postgres database name is required")
	}

	maintenanceDB := cfg.MaintenanceDB
	if maintenanceDB == "" {
		maintenanceDB = "postgres"
	}

	adminDB, err := sqlx.ConnectContext(ctx, "postgres", cfg.dsn(maintenanceDB))
	if err != nil {
		return fmt.Errorf("failed to connect to maintenance database: %w", err)
	}
	defer adminDB.Close()

	var exists bool
	if err := adminDB.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, cfg.DBName); err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}
	if exists {
		slog.Info("База данных уже существует", slog.String("db_name", cfg.DBName))
		return nil
	}

	slog.Info("База данных не найдена, создаем автоматически", slog.String("db_name", cfg.DBName))
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(cfg.DBName)); err != nil {
		if isDuplicateDatabase(err) {
			slog.Info("База данных уже создана параллельным процессом", slog.String("db_name", cfg.DBName))
			return nil
		}
		return fmt.Errorf("failed to create database %q: %w", cfg.DBName, err)
	}

	slog.Info("База данных успешно создана", slog.String("db_name", cfg.DBName))
	return nil
}

// isDuplicateDatabase определяет ошибку PostgreSQL о повторном создании базы.
func isDuplicateDatabase(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "42P04"
}

// dsn собирает строку подключения к PostgreSQL для указанной базы данных.
func (cfg Config) dsn(dbName string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, dbName, cfg.SSLMode)
}
