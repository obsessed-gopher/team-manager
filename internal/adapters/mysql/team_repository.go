package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/zyablitskiy/team-manager/internal/models"
)

// CreateTeamWithOwner создаёт команду и добавляет создателя как owner в одной транзакции.
func (s *Store) CreateTeamWithOwner(ctx context.Context, name string, ownerID int64) (*models.Team, error) {
	var team models.Team
	err := s.WithTx(ctx, func(q Querier) error {
		res, err := q.ExecContext(ctx, `INSERT INTO teams (name, created_by) VALUES (?, ?)`, name, ownerID)
		if err != nil {
			return fmt.Errorf("insert team: %w", err)
		}

		teamID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("team last id: %w", err)
		}

		_, err = q.ExecContext(ctx,
			`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`,
			teamID, ownerID, models.RoleOwner)
		if err != nil {
			return fmt.Errorf("insert owner membership: %w", err)
		}

		return q.QueryRowContext(ctx,
			`SELECT id, name, created_by, created_at FROM teams WHERE id = ?`, teamID).
			Scan(&team.ID, &team.Name, &team.CreatedBy, &team.CreatedAt)
	})
	if err != nil {
		return nil, err
	}

	return &team, nil
}

// ListTeamsForUser возвращает команды, в которых состоит пользователь.
func (s *Store) ListTeamsForUser(ctx context.Context, userID int64) ([]*models.Team, error) {
	const query = `
		SELECT t.id, t.name, t.created_by, t.created_at
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = ?
		ORDER BY t.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var teams []*models.Team
	for rows.Next() {
		var t models.Team
		if err = rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}

		teams = append(teams, &t)
	}

	return teams, rows.Err()
}

// GetMembership возвращает роль пользователя в команде или ErrNotFound.
func (s *Store) GetMembership(ctx context.Context, teamID, userID int64) (*models.TeamMember, error) {
	const query = `
		SELECT team_id, user_id, role, joined_at
		FROM team_members WHERE team_id = ? AND user_id = ?`

	var m models.TeamMember
	err := s.db.QueryRowContext(ctx, query, teamID, userID).
		Scan(&m.TeamID, &m.UserID, &m.Role, &m.JoinedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get membership: %w", err)
	}

	return &m, nil
}

// AddMember добавляет пользователя в команду с указанной ролью.
// Идемпотентно по (team_id, user_id) — при повторе обновляет роль.
func (s *Store) AddMember(ctx context.Context, teamID, userID int64, role models.Role) error {
	const query = `
		INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE role = VALUES(role)`

	if _, err := s.db.ExecContext(ctx, query, teamID, userID, role); err != nil {
		return fmt.Errorf("add member: %w", err)
	}

	return nil
}

// GetTeamByID возвращает команду по ID.
func (s *Store) GetTeamByID(ctx context.Context, teamID int64) (*models.Team, error) {
	const query = `SELECT id, name, created_by, created_at FROM teams WHERE id = ?`

	var t models.Team
	err := s.db.QueryRowContext(ctx, query, teamID).
		Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get team: %w", err)
	}

	return &t, nil
}
