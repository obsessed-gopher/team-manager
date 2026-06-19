package mysql

import (
	"context"
	"fmt"

	"github.com/zyablitskiy/team-manager/internal/models"
)

// TeamStats реализует сложный запрос (а): JOIN 3+ таблиц + агрегация.
// Для каждой команды: название, число участников и число задач в статусе done
// за последние 7 дней.
func (s *Store) TeamStats(ctx context.Context) ([]*models.TeamStats, error) {
	const query = `
		SELECT
			t.id,
			t.name,
			COUNT(DISTINCT tm.user_id) AS members_count,
			COUNT(DISTINCT CASE
				WHEN tk.status = 'done'
				 AND tk.updated_at >= (NOW() - INTERVAL 7 DAY)
				THEN tk.id END) AS done_last_7_days
		FROM teams t
		LEFT JOIN team_members tm ON tm.team_id = t.id
		LEFT JOIN tasks tk        ON tk.team_id = t.id
		GROUP BY t.id, t.name
		ORDER BY done_last_7_days DESC, t.id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("team stats: %w", err)
	}
	defer rows.Close()

	var out []*models.TeamStats
	for rows.Next() {
		var st models.TeamStats
		if err = rows.Scan(&st.TeamID, &st.TeamName, &st.MembersCount, &st.DoneLast7Days); err != nil {
			return nil, fmt.Errorf("scan team stats: %w", err)
		}

		out = append(out, &st)
	}

	return out, rows.Err()
}

// TopCreators реализует сложный запрос (б): оконная функция ROW_NUMBER().
// Топ-3 пользователя по числу созданных задач в каждой команде за последний месяц.
func (s *Store) TopCreators(ctx context.Context) ([]*models.TopCreator, error) {
	const query = `
		SELECT team_id, team_name, user_id, user_name, tasks_created, rnk
		FROM (
			SELECT
				t.id   AS team_id,
				t.name AS team_name,
				u.id   AS user_id,
				u.name AS user_name,
				COUNT(tk.id) AS tasks_created,
				ROW_NUMBER() OVER (
					PARTITION BY t.id
					ORDER BY COUNT(tk.id) DESC, u.id
				) AS rnk
			FROM tasks tk
			JOIN teams t ON t.id = tk.team_id
			JOIN users u ON u.id = tk.created_by
			WHERE tk.created_at >= (NOW() - INTERVAL 1 MONTH)
			GROUP BY t.id, t.name, u.id, u.name
		) ranked
		WHERE rnk <= 3
		ORDER BY team_id, rnk`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("top creators: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []*models.TopCreator
	for rows.Next() {
		var tc models.TopCreator
		if err = rows.Scan(&tc.TeamID, &tc.TeamName, &tc.UserID, &tc.UserName,
			&tc.TasksCreated, &tc.Rank); err != nil {
			return nil, fmt.Errorf("scan top creator: %w", err)
		}

		out = append(out, &tc)
	}

	return out, rows.Err()
}

// IntegrityIssues реализует сложный запрос (в): условие по связанным таблицам.
// Находит задачи, где assignee не является участником команды этой задачи.
func (s *Store) IntegrityIssues(ctx context.Context) ([]*models.IntegrityIssue, error) {
	const query = `
		SELECT tk.id, tk.team_id, tk.assignee_id
		FROM tasks tk
		WHERE tk.assignee_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM team_members tm
			WHERE tm.team_id = tk.team_id
			  AND tm.user_id = tk.assignee_id
		  )
		ORDER BY tk.id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("integrity issues: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []*models.IntegrityIssue
	for rows.Next() {
		var it models.IntegrityIssue
		if err = rows.Scan(&it.TaskID, &it.TeamID, &it.AssigneeID); err != nil {
			return nil, fmt.Errorf("scan integrity issue: %w", err)
		}

		out = append(out, &it)
	}

	return out, rows.Err()
}
