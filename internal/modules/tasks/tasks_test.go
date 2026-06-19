package tasks

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/zyablitskiy/team-manager/internal/adapters/cache"
	"github.com/zyablitskiy/team-manager/internal/adapters/mysql"
	"github.com/zyablitskiy/team-manager/internal/models"
	"github.com/zyablitskiy/team-manager/internal/pkg/httpx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockRepo struct {
	memberships map[[2]int64]models.Role
	tasks       map[int64]*models.Task
	history     map[int64][]*models.TaskHistory
	nextTaskID  int64
	lastUpdate  *mysql.TaskUpdate
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		memberships: map[[2]int64]models.Role{},
		tasks:       map[int64]*models.Task{},
		history:     map[int64][]*models.TaskHistory{},
		nextTaskID:  1,
	}
}

func (m *mockRepo) member(teamID, userID int64, role models.Role) {
	m.memberships[[2]int64{teamID, userID}] = role
}

func (m *mockRepo) GetMembership(_ context.Context, teamID, userID int64) (*models.TeamMember, error) {
	role, ok := m.memberships[[2]int64{teamID, userID}]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return &models.TeamMember{TeamID: teamID, UserID: userID, Role: role}, nil
}

func (m *mockRepo) CreateTask(_ context.Context, t *models.Task) (*models.Task, error) {
	t.ID = m.nextTaskID
	m.nextTaskID++
	stored := *t
	m.tasks[t.ID] = &stored
	return &stored, nil
}

func (m *mockRepo) GetTaskByID(_ context.Context, id int64) (*models.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return t, nil
}

func (m *mockRepo) ListTasks(_ context.Context, f models.TaskFilter) ([]*models.Task, error) {
	var out []*models.Task
	for _, t := range m.tasks {
		if t.TeamID != f.TeamID {
			continue
		}
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (m *mockRepo) UpdateTask(_ context.Context, id, _ int64, upd mysql.TaskUpdate) (*models.Task, error) {
	m.lastUpdate = &upd
	t, ok := m.tasks[id]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	if upd.Status != nil {
		t.Status = *upd.Status
	}
	if upd.Title != nil {
		t.Title = *upd.Title
	}
	if upd.ClearAssignee {
		t.AssigneeID = nil
	} else if upd.AssigneeID != nil {
		t.AssigneeID = upd.AssigneeID
	}
	return t, nil
}

func (m *mockRepo) GetTaskHistory(_ context.Context, taskID int64) ([]*models.TaskHistory, error) {
	return m.history[taskID], nil
}

type mockCache struct {
	store       map[string][]byte
	invalidated int
	setCount    int
	forceHit    []byte
}

func newMockCache() *mockCache { return &mockCache{store: map[string][]byte{}} }

func (m *mockCache) GetTasks(_ context.Context, key string) ([]byte, error) {
	if m.forceHit != nil {
		return m.forceHit, nil
	}
	v, ok := m.store[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return v, nil
}

func (m *mockCache) SetTasks(_ context.Context, key string, data []byte) error {
	m.store[key] = data
	m.setCount++
	return nil
}

func (m *mockCache) InvalidateTeamTasks(_ context.Context, _ int64) error {
	m.invalidated++
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func status(t *testing.T, err error) int {
	t.Helper()
	var de *httpx.Error
	require.ErrorAs(t, err, &de)
	return de.Status
}

func ptr[T any](v T) *T { return &v }

// --- Create ---

func TestCreate_Success(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	c := newMockCache()
	svc := NewService(repo, c, discardLogger())

	task, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: " Do work "})
	require.NoError(t, err)
	assert.Equal(t, "Do work", task.Title)
	assert.Equal(t, models.StatusTodo, task.Status, "status defaults to todo")
	assert.Equal(t, int64(10), task.CreatedBy)
	assert.Equal(t, 1, c.invalidated, "cache must be invalidated after create")
}

func TestCreate_NotAMember(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: "X"})
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestCreate_EmptyTitle(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	svc := NewService(repo, newMockCache(), discardLogger())
	_, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: "  "})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestCreate_InvalidStatus(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	svc := NewService(repo, newMockCache(), discardLogger())
	_, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: "X", Status: "weird"})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestCreate_AssigneeNotInTeam(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember) // actor only
	svc := NewService(repo, newMockCache(), discardLogger())
	_, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: "X", AssigneeID: ptr(int64(77))})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestCreate_AssigneeInTeam(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	repo.member(1, 11, models.RoleMember)
	svc := NewService(repo, newMockCache(), discardLogger())
	task, err := svc.Create(context.Background(), 10, CreateInput{TeamID: 1, Title: "X", AssigneeID: ptr(int64(11))})
	require.NoError(t, err)
	require.NotNil(t, task.AssigneeID)
	assert.Equal(t, int64(11), *task.AssigneeID)
}

// --- List + caching ---

func TestList_RequiresTeamID(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.List(context.Background(), 10, models.TaskFilter{})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestList_NotAMember(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.List(context.Background(), 10, models.TaskFilter{TeamID: 1})
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestList_InvalidStatusFilter(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.List(context.Background(), 10, models.TaskFilter{TeamID: 1, Status: "bad"})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestList_CacheMissThenSet(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Title: "A", Status: models.StatusTodo}
	c := newMockCache()
	svc := NewService(repo, c, discardLogger())

	tasks, err := svc.List(context.Background(), 10, models.TaskFilter{TeamID: 1})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, 1, c.setCount, "cache must be populated on miss")
}

func TestList_CacheHitSkipsRepo(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	// Репозиторий пуст, но кеш возвращает одну задачу — проверяем, что отдаётся кеш.
	c := newMockCache()
	c.forceHit = []byte(`[{"id":99,"team_id":1,"title":"cached","status":"todo"}]`)
	svc := NewService(repo, c, discardLogger())

	tasks, err := svc.List(context.Background(), 10, models.TaskFilter{TeamID: 1})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, int64(99), tasks[0].ID)
	assert.Equal(t, "cached", tasks[0].Title)
}

func TestList_PaginationNormalized(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	c := newMockCache()
	svc := NewService(repo, c, discardLogger())

	_, err := svc.List(context.Background(), 10, models.TaskFilter{TeamID: 1, Limit: 0, Offset: -5})
	require.NoError(t, err)
	// Ключ кеша должен содержать нормализованные limit=20, offset=0.
	_, ok := c.store["team:1:status:any:assignee:any:limit:20:offset:0"]
	assert.True(t, ok, "expected normalized cache key, got keys: %v", c.store)
}

// --- Update ---

func TestUpdate_TaskNotFound(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.Update(context.Background(), 10, 123, mysql.TaskUpdate{})
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

func TestUpdate_NotAMember(t *testing.T) {
	repo := newMockRepo()
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Status: models.StatusTodo}
	svc := NewService(repo, newMockCache(), discardLogger())
	_, err := svc.Update(context.Background(), 10, 1, mysql.TaskUpdate{Status: ptr(models.StatusDone)})
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestUpdate_Success(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Status: models.StatusTodo}
	c := newMockCache()
	svc := NewService(repo, c, discardLogger())

	task, err := svc.Update(context.Background(), 10, 1, mysql.TaskUpdate{Status: ptr(models.StatusDone)})
	require.NoError(t, err)
	assert.Equal(t, models.StatusDone, task.Status)
	assert.Equal(t, 1, c.invalidated)
}

func TestUpdate_AssigneeMustBeMember(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Status: models.StatusTodo}
	svc := NewService(repo, newMockCache(), discardLogger())

	_, err := svc.Update(context.Background(), 10, 1, mysql.TaskUpdate{AssigneeID: ptr(int64(88))})
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestUpdate_UnsetAssigneeRequiresAdmin(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember) // обычный участник
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Status: models.StatusTodo, AssigneeID: ptr(int64(10))}
	svc := NewService(repo, newMockCache(), discardLogger())

	_, err := svc.Update(context.Background(), 10, 1, mysql.TaskUpdate{ClearAssignee: true})
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestUpdate_UnsetAssigneeByOwner(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleOwner)
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1, Status: models.StatusTodo, AssigneeID: ptr(int64(10))}
	svc := NewService(repo, newMockCache(), discardLogger())

	task, err := svc.Update(context.Background(), 10, 1, mysql.TaskUpdate{ClearAssignee: true})
	require.NoError(t, err)
	assert.Nil(t, task.AssigneeID)
}

// --- History ---

func TestHistory_NotAMember(t *testing.T) {
	repo := newMockRepo()
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1}
	svc := NewService(repo, newMockCache(), discardLogger())
	_, err := svc.History(context.Background(), 10, 1)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestHistory_TaskNotFound(t *testing.T) {
	svc := NewService(newMockRepo(), newMockCache(), discardLogger())
	_, err := svc.History(context.Background(), 10, 404)
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

func TestHistory_Success(t *testing.T) {
	repo := newMockRepo()
	repo.member(1, 10, models.RoleMember)
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1}
	repo.history[1] = []*models.TaskHistory{{ID: 1, TaskID: 1, Field: "status"}}
	svc := NewService(repo, newMockCache(), discardLogger())

	hist, err := svc.History(context.Background(), 10, 1)
	require.NoError(t, err)
	assert.Len(t, hist, 1)
}
