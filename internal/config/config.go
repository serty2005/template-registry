package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"templates-registry/internal/pkg/postgres"
)

type Config struct {
	AppAddr       string
	AppVersion    string
	JWTSecret     string
	TokenTTL      time.Duration
	AdminUsername string
	AdminPassword string
	Postgres      postgres.Config
}

// Load загружает переменные окружения и собирает итоговую конфигурацию приложения.
func Load() (Config, error) {
	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppAddr:       envOr("APP_ADDR", ":"+envOr("PORT", "8080")),
		AppVersion:    envOr("APP_VERSION", "1.0"),
		JWTSecret:     envOr("JWT_SECRET", "super_secret_key_change_in_production"),
		TokenTTL:      hoursEnvOr("TOKEN_TTL_HOURS", 24),
		AdminUsername: envOr("ADMIN_USERNAME", "admin"),
		AdminPassword: envOr("ADMIN_PASSWORD", "admin"),
		Postgres: postgres.Config{
			Host:          envOr("POSTGRES_HOST", envOr("PGHOST", "127.0.0.1")),
			Port:          envOr("POSTGRES_PORT", envOr("PGPORT", "5432")),
			User:          envOr("POSTGRES_USER", envOr("PGUSER", "postgres")),
			Password:      envOr("POSTGRES_PASSWORD", envOr("PGPASSWORD", "postgres")),
			DBName:        envOr("POSTGRES_DB", envOr("PGDATABASE", "templates_registry")),
			SSLMode:       envOr("POSTGRES_SSLMODE", "disable"),
			MaintenanceDB: envOr("POSTGRES_MAINTENANCE_DB", "postgres"),
		},
	}

	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET must not be empty")
	}

	return cfg, nil
}

// FindProjectFile ищет файл проекта, поднимаясь вверх по дереву каталогов.
func FindProjectFile(relativePath string) (string, error) {
	if wd, err := os.Getwd(); err == nil {
		if path, ok := findUpFrom(wd, relativePath); ok {
			return path, nil
		}
	}

	if executable, err := os.Executable(); err == nil {
		if path, ok := findUpFrom(filepath.Dir(executable), relativePath); ok {
			return path, nil
		}
	}

	return "", os.ErrNotExist
}

// loadDotEnv загружает настройки из файла .env, если он найден.
func loadDotEnv() error {
	path, err := FindProjectFile(".env")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	return godotenv.Load(path)
}

// findUpFrom поднимается вверх по каталогам в поиске нужного файла.
func findUpFrom(startDir, relativePath string) (string, bool) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

// envOr возвращает значение переменной окружения или запасное значение.
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// hoursEnvOr читает число часов из окружения и переводит его в duration.
func hoursEnvOr(key string, fallback int) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return time.Duration(fallback) * time.Hour
	}

	hours, err := strconv.Atoi(value)
	if err != nil || hours <= 0 {
		return time.Duration(fallback) * time.Hour
	}

	return time.Duration(hours) * time.Hour
}
