package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/obsessed-gopher/team-manager/internal/platform/httpx"
	"github.com/obsessed-gopher/team-manager/internal/platform/metrics"

	"github.com/go-chi/chi/v5"
)

// statusRecorder перехватывает HTTP-статус ответа.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Metrics фиксирует число запросов, ошибок и время ответа в Prometheus.
func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			path := routePattern(r)
			m.Observe(r.Method, path, rec.status, time.Since(start))
		})
	}
}

// RequestLogger логирует каждый HTTP-запрос: метод, маршрут, статус и длительность.
// Уровень зависит от статуса: 5xx — Error, 4xx — Warn, остальное — Info.
// Служебные эндпоинты (/health, /metrics) логируются на Debug, чтобы не шуметь.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			level := levelForStatus(rec.status, r.URL.Path)
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", routePattern(r)),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// levelForStatus выбирает уровень логирования по статусу ответа и пути.
func levelForStatus(status int, path string) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	case path == "/health" || path == "/metrics":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// routePattern возвращает шаблон маршрута (без подстановки id) для стабильных меток.
func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}

	return r.URL.Path
}

// Recover ловит панику в обработчиках, логирует её и отдаёт 500.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"error", rec,
						"path", r.URL.Path,
						"stack", string(debug.Stack()))
					httpx.Fail(w, httpx.NewError(http.StatusInternalServerError, "internal server error"))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
