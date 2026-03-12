package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// txKey используется как ключ для хранения транзакции в контексте.
type txKey struct{}

// TransactionManager реализует выполнение функций внутри PostgreSQL-транзакции.
type TransactionManager struct {
	db *sqlx.DB
}

// NewTransactionManager создает менеджер транзакций.
func NewTransactionManager(db *sqlx.DB) *TransactionManager {
	return &TransactionManager{db: db}
}

// Do начинает транзакцию, выполняет функцию и коммитит или откатывает изменения.
func (tm *TransactionManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := tm.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Передаем транзакцию через контекст, чтобы репозитории использовали единый executor.
	txCtx := context.WithValue(ctx, txKey{}, tx)

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
