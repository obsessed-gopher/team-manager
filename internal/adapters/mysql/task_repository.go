package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/obsessed-gopher/team-manager/internal/models"
)

// CreateTask создаёт задачу и стартовую запись истории в одной транзакции.
func (s *Store) CreateTask(ctx context.Context, t *models.Task) (*models.Task, error) {
	err := s.WithTx(ctx, func(q Querier) error {
		res, err := q.ExecContext(ctx,
			`INSERT INTO tasks (team_id, title, description, status, assignee_id, created_by)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			t.TeamID, t.Title, t.Description, t.Status, t.AssigneeID, t.CreatedBy)
		if err != nil {
			return fmt.Errorf("insert task: %w", err)
		}

		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("task last id: %w", err)
		}

		t.ID = id

		if _, err = q.ExecContext(ctx,
			`INSERT INTO task_history (task_id, changed_by, field, old_value, new_value)
			 VALUES (?, ?, 'created', '', ?)`,
			id, t.CreatedBy, string(t.Status)); err != nil {
			return fmt.Errorf("insert history: %w", err)
		}

		return q.QueryRowContext(ctx,
			`SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
			 FROM tasks WHERE id = ?`, id).
			Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status,
				&t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	})
	if err != nil {
		return nil, err
	}

	return t, nil
}

// GetTaskByID возвращает задачу по ID.
func (s *Store) GetTaskByID(ctx context.Context, id int64) (*models.Task, error) {
	const query = `
		SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
		FROM tasks WHERE id = ?`

	var t models.Task
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status,
			&t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get task: %w", err)
	}

	return &t, nil
}

// ListTasks возвращает задачи по фильтру с пагинацией на уровне БД (LIMIT/OFFSET).
func (s *Store) ListTasks(ctx context.Context, f models.TaskFilter) ([]*models.Task, error) {
	var (
		conds = []string{"team_id = ?"}
		args  = []any{f.TeamID}
	)

	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}

	if f.AssigneeID != nil {
		conds = append(conds, "assignee_id = ?")
		args = append(args, *f.AssigneeID)
	}

	// Безопасно: %s заполняется только статичными предикатами выше,
	// все пользовательские значения идут плейсхолдерами через args.
	const tmpl = `
		SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
		FROM tasks
		WHERE %s
		ORDER BY id DESC
		LIMIT ? OFFSET ?`
	query := fmt.Sprintf(tmpl, strings.Join(conds, " AND ")) //nolint:gosec // см. комментарий выше
	args = append(args, f.Limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var tasks []*models.Task
	for rows.Next() {
		var t models.Task
		if err = rows.Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status,
			&t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}

		tasks = append(tasks, &t)
	}

	return tasks, rows.Err()
}

// TaskUpdate описывает изменяемые поля задачи. nil-поля не меняются.
type TaskUpdate struct {
	Title         *string
	Description   *string
	Status        *models.TaskStatus
	AssigneeID    *int64
	ClearAssignee bool
}

// UpdateTask применяет изменения к задаче и пишет историю по каждому изменённому полю.
func (s *Store) UpdateTask(ctx context.Context, id, changedBy int64, upd TaskUpdate) (*models.Task, error) {
	var result *models.Task
	err := s.WithTx(ctx, func(q Querier) error {
		// Блокируем строку и читаем текущее состояние.
		var cur models.Task
		err := q.QueryRowContext(ctx,
			`SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
			 FROM tasks WHERE id = ? FOR UPDATE`, id).
			Scan(&cur.ID, &cur.TeamID, &cur.Title, &cur.Description, &cur.Status,
				&cur.AssigneeID, &cur.CreatedBy, &cur.CreatedAt, &cur.UpdatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}

			return fmt.Errorf("select for update: %w", err)
		}

		var (
			sets    []string
			args    []any
			history []models.TaskHistory
		)

		addHistory := func(field, oldVal, newVal string) {
			history = append(history, models.TaskHistory{
				Field: field, OldValue: oldVal, NewValue: newVal,
			})
		}

		if upd.Title != nil && *upd.Title != cur.Title {
			sets = append(sets, "title = ?")
			args = append(args, *upd.Title)
			addHistory("title", cur.Title, *upd.Title)
		}

		if upd.Description != nil && *upd.Description != cur.Description {
			sets = append(sets, "description = ?")
			args = append(args, *upd.Description)
			addHistory("description", cur.Description, *upd.Description)
		}

		if upd.Status != nil && *upd.Status != cur.Status {
			sets = append(sets, "status = ?")
			args = append(args, *upd.Status)
			addHistory("status", string(cur.Status), string(*upd.Status))
		}

		if upd.ClearAssignee {
			if cur.AssigneeID != nil {
				sets = append(sets, "assignee_id = NULL")
				addHistory("assignee_id", fmt.Sprintf("%d", *cur.AssigneeID), "")
			}
		} else if upd.AssigneeID != nil && (cur.AssigneeID == nil || *cur.AssigneeID != *upd.AssigneeID) {
			sets = append(sets, "assignee_id = ?")
			args = append(args, *upd.AssigneeID)
			oldVal := ""

			if cur.AssigneeID != nil {
				oldVal = fmt.Sprintf("%d", *cur.AssigneeID)
			}

			addHistory("assignee_id", oldVal, fmt.Sprintf("%d", *upd.AssigneeID))
		}

		// Нет изменений — возвращаем текущее состояние без записи в историю.
		if len(sets) == 0 {
			result = &cur
			return nil
		}

		query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(sets, ", "))
		args = append(args, id)
		if _, err = q.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("update task: %w", err)
		}

		for _, h := range history {
			if _, err = q.ExecContext(ctx,
				`INSERT INTO task_history (task_id, changed_by, field, old_value, new_value)
				 VALUES (?, ?, ?, ?, ?)`,
				id, changedBy, h.Field, h.OldValue, h.NewValue); err != nil {
				return fmt.Errorf("insert history: %w", err)
			}
		}

		var t models.Task
		if err = q.QueryRowContext(ctx,
			`SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
			 FROM tasks WHERE id = ?`, id).
			Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status,
				&t.AssigneeID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("reload task: %w", err)
		}

		result = &t

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetTaskHistory возвращает историю изменений задачи (по убыванию времени).
func (s *Store) GetTaskHistory(ctx context.Context, taskID int64) ([]*models.TaskHistory, error) {
	const query = `
		SELECT id, task_id, changed_by, field, old_value, new_value, changed_at
		FROM task_history WHERE task_id = ? ORDER BY id DESC`

	rows, err := s.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []*models.TaskHistory
	for rows.Next() {
		var h models.TaskHistory
		if err = rows.Scan(&h.ID, &h.TaskID, &h.ChangedBy, &h.Field,
			&h.OldValue, &h.NewValue, &h.ChangedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		items = append(items, &h)
	}

	return items, rows.Err()
}
