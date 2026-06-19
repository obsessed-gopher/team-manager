// Package main - единая точка входа в сервис.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/obsessed-gopher/team-manager/internal/adapters/cache"
	"github.com/obsessed-gopher/team-manager/internal/adapters/email"
	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql"
	"github.com/obsessed-gopher/team-manager/internal/adapters/mysql/conn"
	httpapp "github.com/obsessed-gopher/team-manager/internal/app/http"
	"github.com/obsessed-gopher/team-manager/internal/config"
	"github.com/obsessed-gopher/team-manager/internal/modules/auth"
	"github.com/obsessed-gopher/team-manager/internal/modules/comments"
	"github.com/obsessed-gopher/team-manager/internal/modules/tasks"
	"github.com/obsessed-gopher/team-manager/internal/modules/teams"
	"github.com/obsessed-gopher/team-manager/internal/platform/jwt"
	"github.com/obsessed-gopher/team-manager/internal/platform/logger"
)

func main() {
	configPath := flag.String("config", os.Getenv("TM_CONFIG_PATH"), "путь к YAML-конфигу (опционально)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.LogLevel)
	ctx := context.Background()

	// MySQL (connection pooling внутри).
	db, err := conn.New(ctx, &cfg.MySQL)
	if err != nil {
		log.Error("mysql connect failed", "error", err)
		os.Exit(1)
	}
	store := mysql.NewStore(db)

	// Redis.
	redisStore, err := cache.NewStore(ctx, &cfg.Redis)
	if err != nil {
		log.Error("redis connect failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err = store.Close(); err != nil {
			log.Error("store close failed", "error", err)
		}
	}()

	// Внешний email-сервис с circuit breaker.
	emailSvc := email.NewService(&cfg.Email, log)

	// JWT.
	jwtManager := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.TTL)

	// Бизнес-модули.
	authSvc := auth.NewService(store, jwtManager)
	teamsSvc := teams.NewService(store, store, emailSvc)
	tasksSvc := tasks.NewService(store, redisStore, log)
	commentsSvc := comments.NewService(store)

	srv := httpapp.NewServer(httpapp.Deps{
		Config:    cfg,
		Logger:    log,
		JWT:       jwtManager,
		DB:        db,
		Auth:      authSvc,
		Teams:     teamsSvc,
		Tasks:     tasksSvc,
		Comments:  commentsSvc,
		Analytics: store,
	})

	// Запуск сервера в фоне.
	errCh := make(chan error, 1)
	go func() {
		if err = srv.Run(); err != nil {
			errCh <- err
		}
	}()

	// Ожидание сигнала остановки.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err = <-errCh:
		log.Error("http server error", "error", err)
	case sig := <-stop:
		log.Info("shutdown signal received", "signal", sig.String())
	}

	// Graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.TimeOuts.GracefulShutdown)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("graceful shutdown failed", "error", err)
	}

	if err = redisStore.Close(); err != nil {
		log.Error("redis close failed", "error", err)
	}
	if err = store.Close(); err != nil {
		log.Error("mysql close failed", "error", err)
	}

	log.Info("service stopped")
}
