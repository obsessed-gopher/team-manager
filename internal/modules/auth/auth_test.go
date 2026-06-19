package auth

import (
	"context"
	"testing"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// --- mocks ---

type mockUserRepo struct {
	users   map[string]*models.User
	nextID  int64
	created *models.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: map[string]*models.User{}, nextID: 1}
}

func (m *mockUserRepo) CreateUser(_ context.Context, u *models.User) (int64, error) {
	id := m.nextID
	m.nextID++
	stored := *u
	stored.ID = id
	m.users[u.Email] = &stored
	m.created = &stored
	return id, nil
}

func (m *mockUserRepo) GetUserByEmail(_ context.Context, email string) (*models.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, mysql.ErrNotFound
	}
	return u, nil
}

type mockTokens struct{ called bool }

func (m *mockTokens) Generate(_ int64, email string) (string, error) {
	m.called = true
	return "token-for-" + email, nil
}

func httpStatus(t *testing.T, err error) int {
	t.Helper()
	var de *httpx.Error
	require.ErrorAs(t, err, &de)
	return de.Status
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(repo, &mockTokens{}, nil)

	user, err := svc.Register(context.Background(), "  Alice@Example.com ", " Alice ", "password123")
	require.NoError(t, err)
	assert.Equal(t, int64(1), user.ID)
	assert.Equal(t, "alice@example.com", user.Email, "email must be normalized")
	assert.Equal(t, "Alice", user.Name, "name must be trimmed")

	// Пароль должен храниться как bcrypt-хэш, а не в открытом виде.
	require.NotEmpty(t, repo.created.PasswordHash)
	assert.NotEqual(t, "password123", repo.created.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte("password123")))
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(repo, &mockTokens{}, nil)

	_, err := svc.Register(context.Background(), "bob@example.com", "Bob", "password123")
	require.NoError(t, err)

	_, err = svc.Register(context.Background(), "bob@example.com", "Bob2", "password123")
	require.Error(t, err)
	assert.Equal(t, 409, httpStatus(t, err))
}

func TestRegister_Validation(t *testing.T) {
	cases := []struct {
		name, email, uname, pass string
	}{
		{"invalid email", "not-an-email", "Bob", "password123"},
		{"short password", "bob@example.com", "Bob", "short"},
		{"empty name", "bob@example.com", "  ", "password123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewService(newMockUserRepo(), &mockTokens{}, nil)
			_, err := svc.Register(context.Background(), c.email, c.uname, c.pass)
			require.Error(t, err)
			assert.Equal(t, 400, httpStatus(t, err))
		})
	}
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	repo := newMockUserRepo()
	tokens := &mockTokens{}
	svc := NewService(repo, tokens, nil)

	_, err := svc.Register(context.Background(), "carol@example.com", "Carol", "password123")
	require.NoError(t, err)

	token, user, err := svc.Login(context.Background(), "CAROL@example.com", "password123")
	require.NoError(t, err)
	assert.Equal(t, "token-for-carol@example.com", token)
	assert.Equal(t, "carol@example.com", user.Email)
	assert.True(t, tokens.called)
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(repo, &mockTokens{}, nil)
	_, err := svc.Register(context.Background(), "dave@example.com", "Dave", "password123")
	require.NoError(t, err)

	_, _, err = svc.Login(context.Background(), "dave@example.com", "wrong-password")
	require.Error(t, err)
	assert.Equal(t, 401, httpStatus(t, err))
}

func TestLogin_UnknownUser(t *testing.T) {
	svc := NewService(newMockUserRepo(), &mockTokens{}, nil)
	_, _, err := svc.Login(context.Background(), "ghost@example.com", "password123")
	require.Error(t, err)
	assert.Equal(t, 401, httpStatus(t, err))
}
