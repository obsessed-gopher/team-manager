// Package auth содержит бизнес-логику регистрации и аутентификации.
package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"

	"golang.org/x/crypto/bcrypt"
)

// UserRepository — зависимость для доступа к пользователям (определена у потребителя).
type UserRepository interface {
	CreateUser(ctx context.Context, u *models.User) (int64, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
}

// TokenIssuer выпускает JWT-токены.
type TokenIssuer interface {
	Generate(userID int64, email string) (string, error)
}

// Service — сервис аутентификации.
type Service struct {
	repo   UserRepository
	tokens TokenIssuer
}

// NewService создаёт сервис аутентификации.
func NewService(repo UserRepository, tokens TokenIssuer) *Service {
	return &Service{repo: repo, tokens: tokens}
}

// Register регистрирует пользователя и возвращает его модель.
func (s *Service) Register(ctx context.Context, email, name, password string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	if err := validateCredentials(email, password); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, httpx.BadRequest("name is required")
	}

	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return nil, httpx.Conflict("user with this email already exists")
	} else if !errors.Is(err, mysql.ErrNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &models.User{Email: email, Name: name, PasswordHash: string(hash)}

	id, err := s.repo.CreateUser(ctx, u)
	if err != nil {
		return nil, err
	}

	u.ID = id

	return u, nil
}

// Login проверяет учётные данные и возвращает подписанный токен.
func (s *Service) Login(ctx context.Context, email, password string) (string, *models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	u, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return "", nil, httpx.Unauthorized("invalid email or password")
		}

		return "", nil, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", nil, httpx.Unauthorized("invalid email or password")
	}

	token, err := s.tokens.Generate(u.ID, u.Email)
	if err != nil {
		return "", nil, err
	}

	return token, u, nil
}

func validateCredentials(email, password string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return httpx.BadRequest("invalid email")
	}

	if len(password) < 8 {
		return httpx.BadRequest("password must be at least 8 characters")
	}

	return nil
}
