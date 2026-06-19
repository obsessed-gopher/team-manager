// Package metrics регистрирует и предоставляет Prometheus-метрики сервиса.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics — набор метрик HTTP-слоя.
type Metrics struct {
	requestsTotal *prometheus.CounterVec
	errorsTotal   *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// New регистрирует метрики в переданном реестре (или в default, если nil).
func New(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	const (
		labelMethod = "method"
		labelPath   = "path"
		labelStatus = "status"
	)

	factory := promauto.With(reg)

	return &Metrics{
		requestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Общее число HTTP-запросов.",
		}, []string{labelMethod, labelPath, labelStatus}),
		errorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Число HTTP-запросов, завершившихся ошибкой (status >= 400).",
		}, []string{labelMethod, labelPath, labelStatus}),
		duration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Время ответа HTTP-запросов в секундах.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelMethod, labelPath}),
	}
}

// Observe фиксирует завершённый запрос.
func (m *Metrics) Observe(method, path string, status int, elapsed time.Duration) {
	statusStr := strconv.Itoa(status)
	m.requestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.duration.WithLabelValues(method, path).Observe(elapsed.Seconds())
	if status >= 400 {
		m.errorsTotal.WithLabelValues(method, path, statusStr).Inc()
	}
}
