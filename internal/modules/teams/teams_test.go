package teams

import (
	"context"
	"errors"
	"testing"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockRepo struct {
	memberships map[[2]int64]models.Role
	teams       map[int64]*models.Team
	added       []models.TeamMember
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		memberships: map[[2]int64]models.Role{},
		teams:       map[int64]*models.Team{},
	}
}

func (m *mockRepo) CreateTeamWithOwner(_ context.Context, name string, ownerID int64) (*models.Team, error) {
	id := int64(len(m.teams) + 1)
	t := &models.Team{ID: id, Name: name, CreatedBy: ownerID}
	m.teams[id] = t
	m.memberships[[2]int64{id, ownerID}] = models.RoleOwner
	return t, nil
}

func (m *mockRepo) ListTeamsForUser(_ context.Context, userID int64) ([]*models.Team, error) {
	var out []*models.Team
	for key, t := range m.teams {
		if _, ok := m.memberships[[2]int64{key, userID}]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *mockRepo) GetTeamByID(_ context.Context, teamID int64) (*models.Team, error) {
	t, ok := m.teams[teamID]
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

func (m *mockRepo) AddMember(_ context.Context, teamID, userID int64, role models.Role) error {
	m.memberships[[2]int64{teamID, userID}] = role
	m.added = append(m.added, models.TeamMember{TeamID: teamID, UserID: userID, Role: role})
	return nil
}

type mockUsers struct{ byEmail map[string]*models.User }

func (m *mockUsers) GetUserByEmail(_ context.Context, email string) (*models.User, error) {
	u, ok := m.byEmail[email]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return u, nil
}

type mockEmail struct {
	err    error
	called int
}

func (m *mockEmail) SendInvite(_ context.Context, _, _ string) error {
	m.called++
	return m.err
}

func status(t *testing.T, err error) int {
	t.Helper()
	var de *httpx.Error
	require.ErrorAs(t, err, &de)
	return de.Status
}

func setup() (*Service, *mockRepo, *mockUsers, *mockEmail) {
	repo := newMockRepo()
	users := &mockUsers{byEmail: map[string]*models.User{
		"invitee@example.com": {ID: 42, Email: "invitee@example.com", Name: "Invitee"},
	}}
	email := &mockEmail{}
	return NewService(repo, users, email), repo, users, email
}

// --- Create ---

func TestCreate_Success(t *testing.T) {
	svc, _, _, _ := setup()
	team, err := svc.Create(context.Background(), "  Backend  ", 1)
	require.NoError(t, err)
	assert.Equal(t, "Backend", team.Name)
	assert.Equal(t, int64(1), team.CreatedBy)
}

func TestCreate_EmptyName(t *testing.T) {
	svc, _, _, _ := setup()
	_, err := svc.Create(context.Background(), "   ", 1)
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

// --- Invite ---

func TestInvite_OwnerAddsMember(t *testing.T) {
	svc, repo, _, email := setup()
	team, _ := svc.Create(context.Background(), "Team", 1) // user 1 = owner

	res, err := svc.Invite(context.Background(), team.ID, 1, "invitee@example.com", models.RoleMember)
	require.NoError(t, err)
	assert.True(t, res.EmailDelivered)
	assert.Equal(t, int64(42), res.Member.UserID)
	assert.Equal(t, models.RoleMember, repo.memberships[[2]int64{team.ID, 42}])
	assert.Equal(t, 1, email.called)
}

func TestInvite_MemberForbidden(t *testing.T) {
	svc, repo, _, _ := setup()
	team, _ := svc.Create(context.Background(), "Team", 1)
	// user 7 — обычный участник, не может приглашать.
	repo.memberships[[2]int64{team.ID, 7}] = models.RoleMember

	_, err := svc.Invite(context.Background(), team.ID, 7, "invitee@example.com", models.RoleMember)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestInvite_NotAMemberForbidden(t *testing.T) {
	svc, _, _, _ := setup()
	team, _ := svc.Create(context.Background(), "Team", 1)

	_, err := svc.Invite(context.Background(), team.ID, 99, "invitee@example.com", models.RoleMember)
	require.Error(t, err)
	assert.Equal(t, 403, status(t, err))
}

func TestInvite_InvalidRole(t *testing.T) {
	svc, _, _, _ := setup()
	team, _ := svc.Create(context.Background(), "Team", 1)

	_, err := svc.Invite(context.Background(), team.ID, 1, "invitee@example.com", models.RoleOwner)
	require.Error(t, err)
	assert.Equal(t, 400, status(t, err))
}

func TestInvite_UnknownInvitee(t *testing.T) {
	svc, _, _, _ := setup()
	team, _ := svc.Create(context.Background(), "Team", 1)

	_, err := svc.Invite(context.Background(), team.ID, 1, "ghost@example.com", models.RoleMember)
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

func TestInvite_EmailFailureStillAddsMember(t *testing.T) {
	svc, repo, _, email := setup()
	email.err = errors.New("email service unavailable")
	team, _ := svc.Create(context.Background(), "Team", 1)

	res, err := svc.Invite(context.Background(), team.ID, 1, "invitee@example.com", models.RoleAdmin)
	require.NoError(t, err, "сбой email не должен отменять добавление участника")
	assert.False(t, res.EmailDelivered)
	assert.Equal(t, models.RoleAdmin, repo.memberships[[2]int64{team.ID, 42}])
}

func TestInvite_AdminCanInvite(t *testing.T) {
	svc, repo, _, _ := setup()
	team, _ := svc.Create(context.Background(), "Team", 1)
	repo.memberships[[2]int64{team.ID, 5}] = models.RoleAdmin

	res, err := svc.Invite(context.Background(), team.ID, 5, "invitee@example.com", "")
	require.NoError(t, err)
	assert.Equal(t, models.RoleMember, res.Member.Role, "пустая роль по умолчанию = member")
}

func TestInvite_TeamNotFound(t *testing.T) {
	svc, repo, _, _ := setup()
	// Членство есть, но самой команды нет — путь mapNotFound -> 404.
	repo.memberships[[2]int64{999, 1}] = models.RoleOwner

	_, err := svc.Invite(context.Background(), 999, 1, "invitee@example.com", models.RoleMember)
	require.Error(t, err)
	assert.Equal(t, 404, status(t, err))
}

// --- List ---

func TestList_ReturnsEmptySliceNotNil(t *testing.T) {
	svc, _, _, _ := setup()
	teams, err := svc.List(context.Background(), 555)
	require.NoError(t, err)
	assert.NotNil(t, teams)
	assert.Empty(t, teams)
}
