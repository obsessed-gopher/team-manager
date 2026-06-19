package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/obsessed-gopher/team-manager/internal/models"
)

// CreateComment сохраняет комментарий и возвращает его ID.
func (s *Store) CreateComment(ctx context.Context, c *models.TaskComment) (int64, error) {
	const query = `INSERT INTO task_comments (task_id, user_id, body) VALUES (?, ?, ?)`

	res, err := s.db.ExecContext(ctx, query, c.TaskID, c.UserID, c.Body)
	if err != nil {
		return 0, fmt.Errorf("create comment: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create comment last id: %w", err)
	}

	return id, nil
}

// GetCommentByID возвращает комментарий по ID.
func (s *Store) GetCommentByID(ctx context.Context, id int64) (*models.TaskComment, error) {
	const query = `SELECT id, task_id, user_id, body, created_at FROM task_comments WHERE id = ?`

	var c models.TaskComment
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&c.ID, &c.TaskID, &c.UserID, &c.Body, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get comment: %w", err)
	}

	return &c, nil
}

// ListComments возвращает комментарии задачи с пагинацией (LIMIT/OFFSET), по возрастанию времени.
func (s *Store) ListComments(ctx context.Context, taskID int64, limit, offset int) ([]*models.TaskComment, error) {
	const query = `
		SELECT id, task_id, user_id, body, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY id ASC
		LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, taskID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskComment
	for rows.Next() {
		var c models.TaskComment
		if err = rows.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Body, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}

		out = append(out, &c)
	}

	return out, rows.Err()
}

// UpdateComment меняет текст комментария.
func (s *Store) UpdateComment(ctx context.Context, id int64, body string) (*models.TaskComment, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE task_comments SET body = ? WHERE id = ?`, body, id); err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	return s.GetCommentByID(ctx, id)
}

// DeleteComment удаляет комментарий по ID.
func (s *Store) DeleteComment(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM task_comments WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}

	return nil
}
