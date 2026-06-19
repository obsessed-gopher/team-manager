// Package mysql содержит реализацию хранилища поверх MySQL.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound возвращается, когда запись не найдена.
var ErrNotFound = errors.New("not found")

// Querier абстрагирует *sql.DB и *sql.Tx — позволяет переиспользовать
// репозиторные методы как вне, так и внутри транзакции.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store — хранилище, инкапсулирующее пул соединений MySQL.
type Store struct {
	db *sql.DB
}

// NewStore создаёт Store поверх готового пула соединений.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// DB возвращает базовый пул (для health-check и закрытия).
func (s *Store) DB() *sql.DB { return s.db }

// Close закрывает пул соединений.
func (s *Store) Close() error { return s.db.Close() }

// WithTx выполняет f в рамках транзакции, откатывая её при ошибке.
func (s *Store) WithTx(ctx context.Context, f func(q Querier) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err = f(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}

		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
