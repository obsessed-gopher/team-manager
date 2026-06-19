package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zyablitskiy/team-manager/internal/pkg/httpx"

	"golang.org/x/time/rate"
)

// RateLimiter ограничивает число запросов на пользователя (или IP для анонимов).
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter создаёт лимитер на rpm запросов в минуту с заданным burst.
func NewRateLimiter(requestsPerMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rps:      rate.Limit(float64(requestsPerMinute) / 60.0),
		burst:    burst,
	}

	go rl.cleanupLoop()

	return rl
}

func (rl *RateLimiter) limiterFor(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[key]
	if !ok {
		lim := rate.NewLimiter(rl.rps, rl.burst)
		rl.visitors[key] = &visitor{limiter: lim, lastSeen: time.Now()}

		return lim
	}

	v.lastSeen = time.Now()

	return v.limiter
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for k, v := range rl.visitors {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(rl.visitors, k)
			}
		}

		rl.mu.Unlock()
	}
}

// Middleware ограничивает запросы. Ключ — user_id (если есть) или remote addr.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientKey(r)
		if !rl.limiterFor(key).Allow() {
			w.Header().Set("Retry-After", "60")
			httpx.Fail(w, httpx.NewError(http.StatusTooManyRequests, "rate limit exceeded"))

			return
		}

		next.ServeHTTP(w, r)
	})
}

func clientKey(r *http.Request) string {
	if uid, ok := UserID(r.Context()); ok {
		return "user:" + strconv.FormatInt(uid, 10)
	}

	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if idx := strings.IndexByte(fwd, ','); idx >= 0 {
			return strings.TrimSpace(fwd[:idx])
		}

		return strings.TrimSpace(fwd)
	}

	return r.RemoteAddr
}
