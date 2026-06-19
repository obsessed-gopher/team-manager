// Package comments содержит бизнес-логику комментариев к задачам.
package comments

import (
	"context"
	"errors"
	"strings"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"
)

// Repository — доступ к данным комментариев и связанным сущностям (определён у потребителя).
type Repository interface {
	CreateComment(ctx context.Context, c *models.TaskComment) (int64, error)
	GetCommentByID(ctx context.Context, id int64) (*models.TaskComment, error)
	ListComments(ctx context.Context, taskID int64, limit, offset int) ([]*models.TaskComment, error)
	UpdateComment(ctx context.Context, id int64, body string) (*models.TaskComment, error)
	DeleteComment(ctx context.Context, id int64) error
	GetTaskByID(ctx context.Context, id int64) (*models.Task, error)
	GetMembership(ctx context.Context, teamID, userID int64) (*models.TeamMember, error)
}

// Service — сервис комментариев.
type Service struct {
	repo Repository
}

// NewService создаёт сервис комментариев.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create добавляет комментарий к задаче. Доступно участнику команды задачи.
func (s *Service) Create(ctx context.Context, actorID, taskID int64, body string) (*models.TaskComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, httpx.BadRequest("comment body is required")
	}

	task, err := s.requireTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if _, err = s.requireMembership(ctx, task.TeamID, actorID); err != nil {
		return nil, err
	}

	c := &models.TaskComment{TaskID: taskID, UserID: actorID, Body: body}

	id, err := s.repo.CreateComment(ctx, c)
	if err != nil {
		return nil, err
	}

	c.ID = id

	return s.repo.GetCommentByID(ctx, id)
}

// List возвращает комментарии задачи с пагинацией. Доступно участнику команды.
func (s *Service) List(ctx context.Context, actorID, taskID int64, limit, offset int) ([]*models.TaskComment, error) {
	limit, offset = normalizePagination(limit, offset)

	task, err := s.requireTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if _, err = s.requireMembership(ctx, task.TeamID, actorID); err != nil {
		return nil, err
	}

	list, err := s.repo.ListComments(ctx, taskID, limit, offset)
	if err != nil {
		return nil, err
	}

	if list == nil {
		list = []*models.TaskComment{}
	}

	return list, nil
}

// Update меняет текст комментария. Доступно только автору.
func (s *Service) Update(ctx context.Context, actorID, commentID int64, body string) (*models.TaskComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, httpx.BadRequest("comment body is required")
	}

	comment, err := s.requireComment(ctx, commentID)
	if err != nil {
		return nil, err
	}

	if comment.UserID != actorID {
		return nil, httpx.Forbidden("only the author can edit this comment")
	}

	return s.repo.UpdateComment(ctx, commentID, body)
}

// Delete удаляет комментарий. Доступно автору или owner/admin команды.
func (s *Service) Delete(ctx context.Context, actorID, commentID int64) error {
	comment, err := s.requireComment(ctx, commentID)
	if err != nil {
		return err
	}

	if comment.UserID != actorID {
		task, err := s.requireTask(ctx, comment.TaskID)
		if err != nil {
			return err
		}

		membership, err := s.requireMembership(ctx, task.TeamID, actorID)
		if err != nil {
			return err
		}

		if !membership.Role.CanInvite() {
			return httpx.Forbidden("only the author, owner or admin can delete this comment")
		}
	}

	return s.repo.DeleteComment(ctx, commentID)
}

func (s *Service) requireTask(ctx context.Context, taskID int64) (*models.Task, error) {
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.NotFound("task not found")
		}

		return nil, err
	}

	return task, nil
}

func (s *Service) requireComment(ctx context.Context, id int64) (*models.TaskComment, error) {
	comment, err := s.repo.GetCommentByID(ctx, id)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.NotFound("comment not found")
		}

		return nil, err
	}

	return comment, nil
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

func normalizePagination(limit, offset int) (int, int) {
	const (
		defaultLimit = 20
		maxLimit     = 100
	)

	if limit <= 0 {
		limit = defaultLimit
	}

	if limit > maxLimit {
		limit = maxLimit
	}

	if offset < 0 {
		offset = 0
	}

	return limit, offset
}
