package comments

import (
	"context"
	"testing"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ---

type mockRepo struct {
	memberships map[[2]int64]models.Role
	tasks       map[int64]*models.Task
	comments    map[int64]*models.TaskComment
	nextID      int64
	deleted     []int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		memberships: map[[2]int64]models.Role{},
		tasks:       map[int64]*models.Task{},
		comments:    map[int64]*models.TaskComment{},
		nextID:      1,
	}
}

func (m *mockRepo) member(teamID, userID int64, role models.Role) {
	m.memberships[[2]int64{teamID, userID}] = role
}

func (m *mockRepo) CreateComment(_ context.Context, c *models.TaskComment) (int64, error) {
	id := m.nextID
	m.nextID++
	stored := *c
	stored.ID = id
	m.comments[id] = &stored
	return id, nil
}

func (m *mockRepo) GetCommentByID(_ context.Context, id int64) (*models.TaskComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return c, nil
}

func (m *mockRepo) ListComments(_ context.Context, taskID int64, _, _ int) ([]*models.TaskComment, error) {
	var out []*models.TaskComment
	for _, c := range m.comments {
		if c.TaskID == taskID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *mockRepo) UpdateComment(_ context.Context, id int64, body string) (*models.TaskComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	c.Body = body
	return c, nil
}

func (m *mockRepo) DeleteComment(_ context.Context, id int64) error {
	delete(m.comments, id)
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *mockRepo) GetTaskByID(_ context.Context, id int64) (*models.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return t, nil
}

func (m *mockRepo) GetMembership(_ context.Context, teamID, userID int64) (*models.TeamMember, error) {
	role, ok := m.memberships[[2]int64{teamID, userID}]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return &models.TeamMember{TeamID: teamID, UserID: userID, Role: role}, nil
}

func status(t *testing.T, err error) int {
	t.Helper()
	var de *httpx.Error
	require.ErrorAs(t, err, &de)
	return de.Status
}

// repo с задачей 1 в команде 1 и участником user 10.
func setup() (*Service, *mockRepo) {
	repo := newMockRepo()
	repo.tasks[1] = &models.Task{ID: 1, TeamID: 1}
	repo.member(1, 10, models.RoleMember)
	return NewService(repo), repo
}

// --- Create ---

func TestCreate_Success(t *testing.T) {
	svc, _ := setup()
	c, err := svc.Create(context.Background(), 10, 1, "  hello  ")
	require.NoError(t, err)
	assert.Equal(t, "hello", c.Body)
	assert.Equal(t, int64(10), c.UserID)
	assert.Equal(t, int64(1), c.TaskID)
}

func TestCreate_EmptyBody(t *testing.T) {
	svc, _ := setup()
	_, err := svc.Create(context.Background(), 10, 1, "   ")
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestCreate_TaskNotFound(t *testing.T) {
	svc, _ := setup()
	_, err := svc.Create(context.Background(), 10, 999, "hi")
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

func TestCreate_NotAMember(t *testing.T) {
	svc, _ := setup()
	_, err := svc.Create(context.Background(), 77, 1, "hi")
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

// --- List ---

func TestList_NotAMember(t *testing.T) {
	svc, _ := setup()
	_, err := svc.List(context.Background(), 77, 1, 20, 0)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestList_EmptyNotNil(t *testing.T) {
	svc, _ := setup()
	list, err := svc.List(context.Background(), 10, 1, 20, 0)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

// --- Update ---

func TestUpdate_OnlyAuthor(t *testing.T) {
	svc, repo := setup()
	repo.member(1, 11, models.RoleMember)
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "old"}

	// Чужой участник не может редактировать.
	_, err := svc.Update(context.Background(), 11, 5, "new")
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))

	// Автор может.
	c, err := svc.Update(context.Background(), 10, 5, "new")
	require.NoError(t, err)
	assert.Equal(t, "new", c.Body)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _ := setup()
	_, err := svc.Update(context.Background(), 10, 999, "x")
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

func TestUpdate_EmptyBody(t *testing.T) {
	svc, repo := setup()
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "old"}
	_, err := svc.Update(context.Background(), 10, 5, "  ")
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

// --- Delete ---

func TestDelete_Author(t *testing.T) {
	svc, repo := setup()
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "x"}

	err := svc.Delete(context.Background(), 10, 5)
	require.NoError(t, err)
	assert.Contains(t, repo.deleted, int64(5))
}

func TestDelete_NonAuthorMemberForbidden(t *testing.T) {
	svc, repo := setup()
	repo.member(1, 11, models.RoleMember)
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "x"}

	err := svc.Delete(context.Background(), 11, 5)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestDelete_AdminCanDeleteOthers(t *testing.T) {
	svc, repo := setup()
	repo.member(1, 12, models.RoleAdmin)
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "x"}

	err := svc.Delete(context.Background(), 12, 5)
	require.NoError(t, err)
	assert.Contains(t, repo.deleted, int64(5))
}

func TestDelete_NonAuthorNonMemberForbidden(t *testing.T) {
	svc, repo := setup()
	repo.comments[5] = &models.TaskComment{ID: 5, TaskID: 1, UserID: 10, Body: "x"}

	err := svc.Delete(context.Background(), 77, 5)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestDelete_NotFound(t *testing.T) {
	svc, _ := setup()
	err := svc.Delete(context.Background(), 10, 999)
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}
