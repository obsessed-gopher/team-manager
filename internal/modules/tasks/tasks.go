// Package tasks содержит бизнес-логику управления задачами.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zyablitskiy/team-manager/internal/adapters/cache"
	"github.com/zyablitskiy/team-manager/internal/adapters/mysql"
	"github.com/zyablitskiy/team-manager/internal/models"
	"github.com/zyablitskiy/team-manager/internal/pkg/httpx"
)

// Repository — доступ к данным задач (определён у потребителя).
type Repository interface {
	CreateTask(ctx context.Context, t *models.Task) (*models.Task, error)
	GetTaskByID(ctx context.Context, id int64) (*models.Task, error)
	ListTasks(ctx context.Context, f models.TaskFilter) ([]*models.Task, error)
	UpdateTask(ctx context.Context, id, changedBy int64, upd mysql.TaskUpdate) (*models.Task, error)
	GetTaskHistory(ctx context.Context, taskID int64) ([]*models.TaskHistory, error)
	GetMembership(ctx context.Context, teamID, userID int64) (*models.TeamMember, error)
}

// Cacher — кеш списков задач команды.
type Cacher interface {
	GetTasks(ctx context.Context, key string) ([]byte, error)
	SetTasks(ctx context.Context, key string, data []byte) error
	InvalidateTeamTasks(ctx context.Context, teamID int64) error
}

// Service — сервис управления задачами.
type Service struct {
	repo   Repository
	cache  Cacher
	logger *slog.Logger
}

// NewService создаёт сервис задач.
func NewService(repo Repository, c Cacher, logger *slog.Logger) *Service {
	return &Service{repo: repo, cache: c, logger: logger}
}

// CreateInput — входные данные для создания задачи.
type CreateInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      models.TaskStatus
	AssigneeID  *int64
}

// Create создаёт задачу. Доступно только участнику команды.
func (s *Service) Create(ctx context.Context, actorID int64, in CreateInput) (*models.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, httpx.BadRequest("title is required")
	}

	if in.Status == "" {
		in.Status = models.StatusTodo
	}

	if !in.Status.Valid() {
		return nil, httpx.BadRequest("invalid status")
	}

	if _, err := s.requireMembership(ctx, in.TeamID, actorID); err != nil {
		return nil, err
	}

	// Назначать можно только участника команды (целостность данных).
	if in.AssigneeID != nil {
		if err := s.requireAssigneeInTeam(ctx, in.TeamID, *in.AssigneeID); err != nil {
			return nil, err
		}
	}

	task, err := s.repo.CreateTask(ctx, &models.Task{
		TeamID:      in.TeamID,
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		AssigneeID:  in.AssigneeID,
		CreatedBy:   actorID,
	})
	if err != nil {
		return nil, err
	}

	s.invalidate(ctx, in.TeamID)

	return task, nil
}

// List возвращает задачи команды по фильтру. Результат кешируется в Redis (TTL).
func (s *Service) List(ctx context.Context, actorID int64, f models.TaskFilter) ([]*models.Task, error) {
	if f.TeamID == 0 {
		return nil, httpx.BadRequest("team_id is required")
	}

	if f.Status != "" && !f.Status.Valid() {
		return nil, httpx.BadRequest("invalid status filter")
	}

	normalizePagination(&f)

	if _, err := s.requireMembership(ctx, f.TeamID, actorID); err != nil {
		return nil, err
	}

	key := cacheKey(f)

	if data, err := s.cache.GetTasks(ctx, key); err == nil {
		var cached []*models.Task
		if jsonErr := json.Unmarshal(data, &cached); jsonErr == nil {
			return cached, nil
		}
	} else if !errors.Is(err, cache.ErrMiss) {
		s.logger.Warn("cache get failed", "error", err)
	}

	tasks, err := s.repo.ListTasks(ctx, f)
	if err != nil {
		return nil, err
	}

	if tasks == nil {
		tasks = []*models.Task{}
	}

	if data, err := json.Marshal(tasks); err == nil {
		if err = s.cache.SetTasks(ctx, key, data); err != nil {
			s.logger.Warn("cache set failed", "error", err)
		}
	}

	return tasks, nil
}

// Update обновляет задачу. Менять может участник команды; перевод в done и смену
// исполнителя ограничиваем правами/целостностью.
func (s *Service) Update(ctx context.Context, actorID, taskID int64, upd mysql.TaskUpdate) (*models.Task, error) {
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.NotFound("task not found")
		}

		return nil, err
	}

	membership, err := s.requireMembership(ctx, task.TeamID, actorID)
	if err != nil {
		return nil, err
	}

	if upd.Status != nil && !upd.Status.Valid() {
		return nil, httpx.BadRequest("invalid status")
	}

	if upd.Title != nil && strings.TrimSpace(*upd.Title) == "" {
		return nil, httpx.BadRequest("title cannot be empty")
	}

	// Новый исполнитель должен быть участником команды.
	if !upd.ClearAssignee && upd.AssigneeID != nil {
		if err = s.requireAssigneeInTeam(ctx, task.TeamID, *upd.AssigneeID); err != nil {
			return nil, err
		}
	}

	// Удалять исполнителя может только owner/admin.
	if upd.ClearAssignee && !membership.Role.CanInvite() {
		return nil, httpx.Forbidden("only owner or admin can unassign a task")
	}

	updated, err := s.repo.UpdateTask(ctx, taskID, actorID, upd)
	if err != nil {
		return nil, err
	}

	s.invalidate(ctx, task.TeamID)

	return updated, nil
}

// History возвращает историю изменений задачи (доступно участнику команды).
func (s *Service) History(ctx context.Context, actorID, taskID int64) ([]*models.TaskHistory, error) {
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, mysql.ErrNotFound) {
			return nil, httpx.NotFound("task not found")
		}

		return nil, err
	}

	if _, err = s.requireMembership(ctx, task.TeamID, actorID); err != nil {
		return nil, err
	}

	history, err := s.repo.GetTaskHistory(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if history == nil {
		history = []*models.TaskHistory{}
	}

	return history, nil
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

func (s *Service) requireAssigneeInTeam(ctx context.Context, teamID, assigneeID int64) error {
	_, err := s.repo.GetMembership(ctx, teamID, assigneeID)
	if errors.Is(err, mysql.ErrNotFound) {
		return httpx.BadRequest("assignee must be a member of the team")
	}

	return err
}

func (s *Service) invalidate(ctx context.Context, teamID int64) {
	if err := s.cache.InvalidateTeamTasks(ctx, teamID); err != nil {
		s.logger.Warn("cache invalidate failed", "team_id", teamID, "error", err)
	}
}

func normalizePagination(f *models.TaskFilter) {
	const (
		defaultLimit = 20
		maxLimit     = 100
	)

	if f.Limit <= 0 {
		f.Limit = defaultLimit
	}

	if f.Limit > maxLimit {
		f.Limit = maxLimit
	}

	if f.Offset < 0 {
		f.Offset = 0
	}
}

func cacheKey(f models.TaskFilter) string {
	assignee := "any"
	if f.AssigneeID != nil {
		assignee = fmt.Sprintf("%d", *f.AssigneeID)
	}

	status := "any"
	if f.Status != "" {
		status = string(f.Status)
	}

	return fmt.Sprintf("team:%d:status:%s:assignee:%s:limit:%d:offset:%d",
		f.TeamID, status, assignee, f.Limit, f.Offset)
}
