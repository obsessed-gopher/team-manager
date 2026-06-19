// Package cache содержит кеширование на базе Redis.
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obsessed-gopher/team-manager/internal/config"

	"github.com/redis/go-redis/v9"
)

// ErrMiss возвращается при отсутствии значения в кеше.
var ErrMiss = errors.New("cache miss")

// Store — обёртка над клиентом Redis.
type Store struct {
	client   *redis.Client
	tasksTTL time.Duration
}

// NewStore создаёт Redis-клиент и проверяет соединение.
func NewStore(ctx context.Context, cfg *config.Redis) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return &Store{client: client, tasksTTL: cfg.TasksTTL}, nil
}

// Close закрывает соединение с Redis.
func (s *Store) Close() error { return s.client.Close() }

// GetTasks возвращает закешированный JSON списка задач команды или ErrMiss.
func (s *Store) GetTasks(ctx context.Context, key string) ([]byte, error) {
	data, err := s.client.Get(ctx, tasksKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrMiss
		}

		return nil, fmt.Errorf("cache get tasks: %w", err)
	}

	return data, nil
}

// SetTasks кеширует JSON списка задач команды с TTL.
func (s *Store) SetTasks(ctx context.Context, key string, data []byte) error {
	if err := s.client.Set(ctx, tasksKey(key), data, s.tasksTTL).Err(); err != nil {
		return fmt.Errorf("cache set tasks: %w", err)
	}

	return nil
}

// InvalidateTeamTasks удаляет все закешированные списки задач команды.
func (s *Store) InvalidateTeamTasks(ctx context.Context, teamID int64) error {
	pattern := fmt.Sprintf("tasks:team:%d:*", teamID)
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()

	for iter.Next(ctx) {
		if err := s.client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("cache invalidate: %w", err)
		}
	}

	return iter.Err()
}

func tasksKey(key string) string {
	return "tasks:" + key
}
