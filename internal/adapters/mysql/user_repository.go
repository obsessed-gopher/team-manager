package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/obsessed-gopher/team-manager/internal/models"
)

// CreateUser сохраняет нового пользователя и возвращает его ID.
func (s *Store) CreateUser(ctx context.Context, u *models.User) (int64, error) {
	const query = `INSERT INTO users (email, name, password_hash) VALUES (?, ?, ?)`

	res, err := s.db.ExecContext(ctx, query, u.Email, u.Name, u.PasswordHash)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create user last id: %w", err)
	}

	return id, nil
}

// GetUserByEmail возвращает пользователя по email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = `SELECT id, email, name, password_hash, created_at FROM users WHERE email = ?`

	var u models.User
	err := s.db.QueryRowContext(ctx, query, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return &u, nil
}

// GetUserByID возвращает пользователя по ID.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	const query = `SELECT id, email, name, password_hash, created_at FROM users WHERE id = ?`

	var u models.User
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &u, nil
}
