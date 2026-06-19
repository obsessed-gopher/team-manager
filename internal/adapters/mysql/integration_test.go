package mysql_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// setupStore поднимает контейнер MySQL, применяет миграции и возвращает Store.
func setupStore(t *testing.T) *mysql.Store {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	ctx := context.Background()

	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", "mysql", "000001_init.up.sql"))
	require.NoError(t, err)
	schema, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	container, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("team_manager"),
		tcmysql.WithUsername("tm"),
		tcmysql.WithPassword("tm"),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(container)
	})

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	require.NoError(t, err)

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.Eventually(t, func() bool {
		return db.PingContext(ctx) == nil
	}, 30*time.Second, time.Second)

	_, err = db.ExecContext(ctx, string(schema))
	require.NoError(t, err)

	return mysql.NewStore(db)
}

func TestIntegration_FullFlow(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	// Пользователи.
	ownerID, err := store.CreateUser(ctx, &models.User{Email: "owner@x.io", Name: "Owner", PasswordHash: "h"})
	require.NoError(t, err)
	memberID, err := store.CreateUser(ctx, &models.User{Email: "member@x.io", Name: "Member", PasswordHash: "h"})
	require.NoError(t, err)
	outsiderID, err := store.CreateUser(ctx, &models.User{Email: "out@x.io", Name: "Out", PasswordHash: "h"})
	require.NoError(t, err)

	// GetUserByEmail / GetUserByID.
	u, err := store.GetUserByEmail(ctx, "owner@x.io")
	require.NoError(t, err)
	assert.Equal(t, ownerID, u.ID)

	_, err = store.GetUserByEmail(ctx, "missing@x.io")
	assert.ErrorIs(t, err, mysql.ErrNotFound)

	// Команда + owner-членство в транзакции.
	team, err := store.CreateTeamWithOwner(ctx, "Platform", ownerID)
	require.NoError(t, err)
	require.NotZero(t, team.ID)

	m, err := store.GetMembership(ctx, team.ID, ownerID)
	require.NoError(t, err)
	assert.Equal(t, models.RoleOwner, m.Role)

	require.NoError(t, store.AddMember(ctx, team.ID, memberID, models.RoleMember))

	teams, err := store.ListTeamsForUser(ctx, memberID)
	require.NoError(t, err)
	require.Len(t, teams, 1)
	assert.Equal(t, "Platform", teams[0].Name)

	// Создание задачи + стартовая запись истории.
	task, err := store.CreateTask(ctx, &models.Task{
		TeamID: team.ID, Title: "Build API", Description: "desc",
		Status: models.StatusTodo, AssigneeID: &memberID, CreatedBy: ownerID,
	})
	require.NoError(t, err)
	require.NotZero(t, task.ID)

	hist, err := store.GetTaskHistory(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	assert.Equal(t, "created", hist[0].Field)

	// Обновление: статус + история по изменённому полю.
	updated, err := store.UpdateTask(ctx, task.ID, memberID, mysql.TaskUpdate{
		Status: statusPtr(models.StatusDone),
	})
	require.NoError(t, err)
	assert.Equal(t, models.StatusDone, updated.Status)

	hist, err = store.GetTaskHistory(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, hist, 2)
	assert.Equal(t, "status", hist[0].Field)
	assert.Equal(t, "todo", hist[0].OldValue)
	assert.Equal(t, "done", hist[0].NewValue)

	// Фильтрация + пагинация.
	list, err := store.ListTasks(ctx, models.TaskFilter{TeamID: team.ID, Status: models.StatusDone, Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)

	list, err = store.ListTasks(ctx, models.TaskFilter{TeamID: team.ID, Status: models.StatusTodo, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, list)

	// --- Комментарии (CRUD) ---
	cID, err := store.CreateComment(ctx, &models.TaskComment{TaskID: task.ID, UserID: memberID, Body: "first"})
	require.NoError(t, err)
	require.NotZero(t, cID)

	got, err := store.GetCommentByID(ctx, cID)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Body)

	_, err = store.CreateComment(ctx, &models.TaskComment{TaskID: task.ID, UserID: ownerID, Body: "second"})
	require.NoError(t, err)

	comments, err := store.ListComments(ctx, task.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, comments, 2)
	assert.Equal(t, "first", comments[0].Body, "по возрастанию времени")

	updated2, err := store.UpdateComment(ctx, cID, "edited")
	require.NoError(t, err)
	assert.Equal(t, "edited", updated2.Body)

	require.NoError(t, store.DeleteComment(ctx, cID))
	_, err = store.GetCommentByID(ctx, cID)
	assert.ErrorIs(t, err, mysql.ErrNotFound)

	comments, err = store.ListComments(ctx, task.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, comments, 1)

	// --- Сложный запрос (а): JOIN 3+ таблиц + агрегация ---
	stats, err := store.TeamStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 2, stats[0].MembersCount)
	assert.Equal(t, 1, stats[0].DoneLast7Days)

	// --- Сложный запрос (б): оконная функция ---
	top, err := store.TopCreators(ctx)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, ownerID, top[0].UserID)
	assert.Equal(t, 1, top[0].TasksCreated)
	assert.Equal(t, 1, top[0].Rank)

	// --- Сложный запрос (в): нарушение целостности ---
	// Назначим задачу на пользователя вне команды напрямую и проверим обнаружение.
	_, err = store.DB().ExecContext(ctx, `UPDATE tasks SET assignee_id = ? WHERE id = ?`, outsiderID, task.ID)
	require.NoError(t, err)

	issues, err := store.IntegrityIssues(ctx)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, task.ID, issues[0].TaskID)
	assert.Equal(t, outsiderID, issues[0].AssigneeID)
}

func statusPtr(s models.TaskStatus) *models.TaskStatus { return &s }
