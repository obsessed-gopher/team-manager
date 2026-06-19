package jwt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndParse(t *testing.T) {
	m := NewManager("secret", time.Hour)

	token, err := m.Generate(42, "user@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "user@example.com", claims.Email)
}

func TestParse_Expired(t *testing.T) {
	m := NewManager("secret", -time.Hour) // токен уже просрочен
	token, err := m.Generate(1, "a@b.c")
	require.NoError(t, err)

	_, err = m.Parse(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestParse_WrongSecret(t *testing.T) {
	issuer := NewManager("secret-a", time.Hour)
	verifier := NewManager("secret-b", time.Hour)

	token, err := issuer.Generate(1, "a@b.c")
	require.NoError(t, err)

	_, err = verifier.Parse(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestParse_Garbage(t *testing.T) {
	m := NewManager("secret", time.Hour)
	_, err := m.Parse("not-a-token")
	require.ErrorIs(t, err, ErrInvalidToken)
}
