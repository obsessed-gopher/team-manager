// Package teams содержит бизнес-логику управления командами.
package teams

import (
	"context"
	"errors"
	"strings"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"
)

// Repository — доступ к данным команд (определён у потребителя).
type Repository interface {
	CreateTeamWithOwner(ctx context.Context, name string, ownerID int64) (*models.Team, error)
	ListTeamsForUser(ctx context.Context, userID int64) ([]*models.Team, error)
	GetTeamByID(ctx context.Context, teamID int64) (*models.Team, error)
	GetMembership(ctx context.Context, teamID, userID int64) (*models.TeamMember, error)
	AddMember(ctx context.Context, teamID, userID int64, role models.Role) error
}

// UserLookup — поиск пользователя по email для приглашения.
type UserLookup interface {
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
}

// EmailSender — внешний сервис уведомлений (с circuit breaker).
type EmailSender interface {
	SendInvite(ctx context.Context, to, teamName string) error
}

// Service — сервис управления командами.
type Service struct {
	repo  Repository
	users UserLookup
	email EmailSender
}

// NewService создаёт сервис команд.
func NewService(repo Repository, users UserLookup, email EmailSender) *Service {
	return &Service{repo: repo, users: users, email: email}
}

// Create создаёт команду; создатель становится owner.
func (s *Service) Create(ctx context.Context, name string, ownerID int64) (*models.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, httpx.BadRequest("team name is required")
	}

	return s.repo.CreateTeamWithOwner(ctx, name, ownerID)
}

// List возвращает команды пользователя.
func (s *Service) List(ctx context.Context, userID int64) ([]*models.Team, error) {
	teams, err := s.repo.ListTeamsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if teams == nil {
		teams = []*models.Team{}
	}

	return teams, nil
}

// InviteResult описывает итог приглашения.
type InviteResult struct {
	Member         *models.TeamMember `json:"member"`
	EmailDelivered bool               `json:"email_delivered"`
}

// Invite добавляет пользователя в команду. Доступно только owner/admin.
// Сбой email-сервиса не отменяет добавление участника.
func (s *Service) Invite(ctx context.Context, teamID, inviterID int64, inviteeEmail string, role models.Role) (*InviteResult, error) {
	inviteeEmail = strings.ToLower(strings.TrimSpace(inviteeEmail))
	if role == "" {
		role = models.RoleMember
	}

	if !role.Valid() || role == models.RoleOwner {
		return nil, httpx.BadRequest("role must be 'admin' or 'member'")
	}

	// Проверка прав приглашающего.
	membership, err := s.requireMembership(ctx, teamID, inviterID)
	if err != nil {
		return nil, err
	}

	if !membership.Role.CanInvite() {
		return nil, httpx.Forbidden("only owner or admin can invite members")
	}

	team, err := s.repo.GetTeamByID(ctx, teamID)
	if err != nil {
		return nil, mapNotFound(err, "team not found")
	}

	invitee, err := s.users.GetUserByEmail(ctx, inviteeEmail)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.NotFound("user with this email not found")
		}

		return nil, err
	}

	if err = s.repo.AddMember(ctx, teamID, invitee.ID, role); err != nil {
		return nil, err
	}

	// Уведомление по email — best effort, защищено circuit breaker.
	emailDelivered := true
	if err = s.email.SendInvite(ctx, invitee.Email, team.Name); err != nil {
		emailDelivered = false
	}

	return &InviteResult{
		Member: &models.TeamMember{
			TeamID: teamID, UserID: invitee.ID, Role: role,
		},
		EmailDelivered: emailDelivered,
	}, nil
}

func (s *Service) requireMembership(ctx context.Context, teamID, userID int64) (*models.TeamMember, error) {
	m, err := s.repo.GetMembership(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.Forbidden("you are not a member of this team")
		}

		return nil, err
	}

	return m, nil
}

func mapNotFound(err error, msg string) error {
	if errors.Is(err, mysql.ErrNotFound) {
		return httpx.NotFound(msg)
	}

	return err
}
