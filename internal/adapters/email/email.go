// Package email содержит мок внешнего email-сервиса, защищённый circuit breaker.
package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/zyablitskiy/team-manager/internal/config"

	"github.com/sony/gobreaker"
)

// ErrServiceUnavailable возвращается, когда circuit breaker разомкнут.
var ErrServiceUnavailable = errors.New("email service unavailable")

// Service — мок отправки email с защитой circuit breaker.
type Service struct {
	cb       *gobreaker.CircuitBreaker
	failRate float64
	latency  time.Duration
	rnd      *rand.Rand
	logger   *slog.Logger
}

// NewService создаёт мок email-сервиса.
func NewService(cfg *config.Email, logger *slog.Logger) *Service {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "email-service",
		MaxRequests: cfg.CBMaxRequests,
		Interval:    cfg.CBInterval,
		Timeout:     cfg.CBTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.CBFailures
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Warn("circuit breaker state changed",
				"name", name, "from", from.String(), "to", to.String())
		},
	})

	return &Service{
		cb:       cb,
		failRate: cfg.FailRate,
		latency:  cfg.Latency,
		rnd:      rand.New(rand.NewSource(time.Now().UnixNano())),
		logger:   logger,
	}
}

// SendInvite отправляет приглашение через защищённый circuit breaker вызов.
func (s *Service) SendInvite(ctx context.Context, to, teamName string) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, s.deliver(ctx, to, teamName)
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ErrServiceUnavailable
	}

	return err
}

// deliver имитирует обращение к внешнему сервису с задержкой и вероятностью сбоя.
func (s *Service) deliver(ctx context.Context, to, teamName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.latency):
	}

	if s.failRate > 0 && s.rnd.Float64() < s.failRate {
		return fmt.Errorf("failed to deliver invite to %s", to)
	}

	s.logger.Info("invite email sent", "to", to, "team", teamName)

	return nil
}
