// Package http содержит HTTP-сервер, маршрутизацию и обработчики REST API.
package http

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/obsessed-gopher/team-manager/internal/app/http/middleware"
	"github.com/obsessed-gopher/team-manager/internal/config"
	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"
	"github.com/obsessed-gopher/team-manager/internal/platform/jwt"
	"github.com/obsessed-gopher/team-manager/internal/platform/metrics"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Deps — зависимости HTTP-сервера.
type Deps struct {
	Config    *config.Config
	Logger    *slog.Logger
	JWT       *jwt.Manager
	DB        *sql.DB
	Auth      AuthService
	Teams     TeamService
	Tasks     TaskService
	Analytics AnalyticsService
}

// Server — HTTP-сервер REST API.
type Server struct {
	cfg       *config.Config
	logger    *slog.Logger
	jwt       *jwt.Manager
	db        *sql.DB
	auth      AuthService
	teams     TeamService
	tasks     TaskService
	analytics AnalyticsService
	metrics   *metrics.Metrics
	rl        *middleware.RateLimiter
	httpSrv   *http.Server
}

// NewServer собирает сервер с маршрутами и middleware.
func NewServer(d Deps) *Server {
	s := &Server{
		cfg:       d.Config,
		logger:    d.Logger,
		jwt:       d.JWT,
		db:        d.DB,
		auth:      d.Auth,
		teams:     d.Teams,
		tasks:     d.Tasks,
		analytics: d.Analytics,
		metrics:   metrics.New(nil),
		rl:        middleware.NewRateLimiter(d.Config.RateLimit.RequestsPerMinute, d.Config.RateLimit.Burst),
	}

	s.httpSrv = &http.Server{
		Addr:         d.Config.HTTP.Address,
		Handler:      s.router(),
		ReadTimeout:  d.Config.HTTP.ReadTimeout,
		WriteTimeout: d.Config.HTTP.WriteTimeout,
		IdleTimeout:  d.Config.HTTP.IdleTimeout,
	}
	return s
}

func (s *Server) router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recover(s.logger))
	r.Use(middleware.Metrics(s.metrics))

	// Служебные эндпоинты.
	r.Get("/health", s.handleHealth)
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		// Публичные эндпоинты с rate limit по IP.
		r.Group(func(r chi.Router) {
			r.Use(s.rl.Middleware)
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
		})

		// Защищённые эндпоинты: auth, затем rate limit по user_id.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(s.jwt))
			r.Use(s.rl.Middleware)

			r.Post("/teams", s.handleCreateTeam)
			r.Get("/teams", s.handleListTeams)
			r.Post("/teams/{id}/invite", s.handleInvite)

			r.Post("/tasks", s.handleCreateTask)
			r.Get("/tasks", s.handleListTasks)
			r.Put("/tasks/{id}", s.handleUpdateTask)
			r.Get("/tasks/{id}/history", s.handleTaskHistory)

			r.Get("/analytics/team-stats", s.handleTeamStats)
			r.Get("/analytics/top-creators", s.handleTopCreators)
			r.Get("/analytics/integrity-issues", s.handleIntegrityIssues)
		})
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	const statusKey = "status"

	if err := s.db.PingContext(ctx); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{statusKey: "db unavailable"})
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{statusKey: "ok"})
}

// Run запускает HTTP-сервер (блокирующе до ошибки или остановки).
func (s *Server) Run() error {
	s.logger.Info("http server started", "address", s.cfg.HTTP.Address)
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// Shutdown корректно останавливает сервер (graceful shutdown).
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http server shutting down")

	return s.httpSrv.Shutdown(ctx)
}
