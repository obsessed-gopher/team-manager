package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/app/http/middleware"
	"github.com/obsessed-gopher/team-manager/internal/models"
	"github.com/obsessed-gopher/team-manager/internal/modules/tasks"
	"github.com/obsessed-gopher/team-manager/internal/modules/teams"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

// teamsKey — ключ JSON-поля со списком команд в ответах.
const teamsKey = "teams"

// AuthService — контракт сервиса аутентификации для HTTP-слоя.
type AuthService interface {
	Register(ctx context.Context, email, name, password string) (*models.User, error)
	Login(ctx context.Context, email, password string) (string, *models.User, error)
}

// TeamService — контракт сервиса команд.
type TeamService interface {
	Create(ctx context.Context, name string, ownerID int64) (*models.Team, error)
	List(ctx context.Context, userID int64) ([]*models.Team, error)
	Invite(ctx context.Context, teamID, inviterID int64, inviteeEmail string, role models.Role) (*teams.InviteResult, error)
}

// TaskService — контракт сервиса задач.
type TaskService interface {
	Create(ctx context.Context, actorID int64, in tasks.CreateInput) (*models.Task, error)
	List(ctx context.Context, actorID int64, f models.TaskFilter) ([]*models.Task, error)
	Update(ctx context.Context, actorID, taskID int64, upd mysql.TaskUpdate) (*models.Task, error)
	History(ctx context.Context, actorID, taskID int64) ([]*models.TaskHistory, error)
}

// AnalyticsService — контракт аналитических запросов.
type AnalyticsService interface {
	TeamStats(ctx context.Context) ([]*models.TeamStats, error)
	TopCreators(ctx context.Context) ([]*models.TopCreator, error)
	IntegrityIssues(ctx context.Context) ([]*models.IntegrityIssue, error)
}

// --- Auth ---

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	user, err := s.auth.Register(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	token, user, err := s.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

// --- Teams ---

type createTeamRequest struct {
	Name string `json:"name"`
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req createTeamRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	team, err := s.teams.Create(r.Context(), req.Name, userID)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, team)
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	teamsList, err := s.teams.List(r.Context(), userID)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{teamsKey: teamsList})
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	teamID, err := pathID(r, "id")
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	var req inviteRequest
	if err = httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	result, err := s.teams.Invite(r.Context(), teamID, userID, req.Email, models.Role(req.Role))
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, result)
}

// --- Tasks ---

type createTaskRequest struct {
	TeamID      int64  `json:"team_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	AssigneeID  *int64 `json:"assignee_id"`
}

type updateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	AssigneeID  *int64  `json:"assignee_id"`
	// UnsetAssignee=true снимает исполнителя.
	UnsetAssignee bool `json:"unset_assignee"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req createTaskRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	task, err := s.tasks.Create(r.Context(), userID, tasks.CreateInput{
		TeamID:      req.TeamID,
		Title:       req.Title,
		Description: req.Description,
		Status:      models.TaskStatus(req.Status),
		AssigneeID:  req.AssigneeID,
	})
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, task)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())

	teamID, err := queryInt64(r, "team_id")
	if err != nil {
		httpx.Fail(w, httpx.BadRequest("team_id is required and must be an integer"))
		return
	}

	filter := models.TaskFilter{
		TeamID: teamID,
		Status: models.TaskStatus(r.URL.Query().Get("status")),
		Limit:  optionalInt(r, "limit", 20),
		Offset: optionalInt(r, "offset", 0),
	}

	if v := r.URL.Query().Get("assignee_id"); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil {
			httpx.Fail(w, httpx.BadRequest("assignee_id must be an integer"))
			return
		}

		filter.AssigneeID = &id
	}

	list, err := s.tasks.List(r.Context(), userID, filter)
	if err != nil {
		httpx.Fail(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tasks":  list,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	taskID, err := pathID(r, "id")
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	var req updateTaskRequest
	if err = httpx.Decode(r, &req); err != nil {
		httpx.Fail(w, err)
		return
	}

	upd := mysql.TaskUpdate{
		Title:         req.Title,
		Description:   req.Description,
		AssigneeID:    req.AssigneeID,
		ClearAssignee: req.UnsetAssignee,
	}

	if req.Status != nil {
		st := models.TaskStatus(*req.Status)
		upd.Status = &st
	}

	task, err := s.tasks.Update(r.Context(), userID, taskID, upd)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, task)
}

func (s *Server) handleTaskHistory(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	taskID, err := pathID(r, "id")
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	history, err := s.tasks.History(r.Context(), userID, taskID)
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"history": history})
}

// --- Analytics ---

func (s *Server) handleTeamStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.analytics.TeamStats(r.Context())
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{teamsKey: stats})
}

func (s *Server) handleTopCreators(w http.ResponseWriter, r *http.Request) {
	top, err := s.analytics.TopCreators(r.Context())
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"top_creators": top})
}

func (s *Server) handleIntegrityIssues(w http.ResponseWriter, r *http.Request) {
	issues, err := s.analytics.IntegrityIssues(r.Context())
	if err != nil {
		httpx.Fail(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"issues": issues})
}

// --- helpers ---

func pathID(r *http.Request, key string) (int64, error) {
	raw := chi.URLParam(r, key)

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid " + key)
	}

	return id, nil
}

func queryInt64(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
}

func optionalInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}

	return n
}
