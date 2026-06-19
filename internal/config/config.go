// Package config содержит конфигурацию сервиса и её загрузку из YAML/ENV.
package config

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config — корневая конфигурация сервиса.
type Config struct {
	Env       string    `yaml:"env" env:"TM_ENV" env-default:"local"`
	LogLevel  string    `yaml:"log_level" env:"TM_LOG_LEVEL" env-default:"INFO"`
	HTTP      HTTP      `yaml:"http"`
	MySQL     MySQL     `yaml:"mysql"`
	Redis     Redis     `yaml:"redis"`
	JWT       JWT       `yaml:"jwt"`
	Email     Email     `yaml:"email"`
	RateLimit RateLimit `yaml:"rate_limit"`
	TimeOuts  TimeOuts  `yaml:"time_outs"`
}

// HTTP — настройки HTTP-сервера.
type HTTP struct {
	Address      string        `yaml:"address" env:"TM_HTTP_ADDRESS" env-default:"0.0.0.0:8080"`
	ReadTimeout  time.Duration `yaml:"read_timeout" env:"TM_HTTP_READ_TIMEOUT" env-default:"15s"`
	WriteTimeout time.Duration `yaml:"write_timeout" env:"TM_HTTP_WRITE_TIMEOUT" env-default:"15s"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" env:"TM_HTTP_IDLE_TIMEOUT" env-default:"60s"`
}

// MySQL — настройки подключения к MySQL и пула соединений.
type MySQL struct {
	Host            string        `yaml:"host" env:"TM_MYSQL_HOST" env-default:"localhost"`
	Port            int           `yaml:"port" env:"TM_MYSQL_PORT" env-default:"3306"`
	User            string        `yaml:"user" env:"TM_MYSQL_USER" env-default:"tm"`
	Password        string        `yaml:"password" env:"TM_MYSQL_PASSWORD" env-default:"tm"`
	DBName          string        `yaml:"db" env:"TM_MYSQL_DB" env-default:"team_manager"`
	MaxOpenConns    int           `yaml:"max_open_conns" env:"TM_MYSQL_MAX_OPEN_CONNS" env-default:"50"`
	MaxIdleConns    int           `yaml:"max_idle_conns" env:"TM_MYSQL_MAX_IDLE_CONNS" env-default:"10"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"TM_MYSQL_CONN_MAX_LIFETIME" env-default:"1h"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" env:"TM_MYSQL_CONN_MAX_IDLE_TIME" env-default:"30m"`
	ConnectTimeout  time.Duration `yaml:"connect_timeout" env:"TM_MYSQL_CONNECT_TIMEOUT" env-default:"5s"`
}

// DSN возвращает строку подключения для go-sql-driver/mysql.
func (m MySQL) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=UTC&timeout=%s",
		m.User, m.Password, m.Host, m.Port, m.DBName, m.ConnectTimeout,
	)
}

// Redis — настройки подключения к Redis.
type Redis struct {
	Address  string        `yaml:"address" env:"TM_REDIS_ADDRESS" env-default:"localhost:6379"`
	Password string        `yaml:"password" env:"TM_REDIS_PASSWORD" env-default:""`
	DB       int           `yaml:"db" env:"TM_REDIS_DB" env-default:"0"`
	PoolSize int           `yaml:"pool_size" env:"TM_REDIS_POOL_SIZE" env-default:"10"`
	TasksTTL time.Duration `yaml:"tasks_ttl" env:"TM_REDIS_TASKS_TTL" env-default:"5m"`
}

// JWT — настройки выпуска токенов.
type JWT struct {
	Secret string        `yaml:"secret" env:"TM_JWT_SECRET" env-default:"super-secret-change-me"`
	TTL    time.Duration `yaml:"ttl" env:"TM_JWT_TTL" env-default:"24h"`
}

// Email — настройки мок-сервиса email и его circuit breaker.
type Email struct {
	FailRate      float64       `yaml:"fail_rate" env:"TM_EMAIL_FAIL_RATE" env-default:"0"`
	Latency       time.Duration `yaml:"latency" env:"TM_EMAIL_LATENCY" env-default:"10ms"`
	CBMaxRequests uint32        `yaml:"cb_max_requests" env:"TM_EMAIL_CB_MAX_REQUESTS" env-default:"3"`
	CBInterval    time.Duration `yaml:"cb_interval" env:"TM_EMAIL_CB_INTERVAL" env-default:"60s"`
	CBTimeout     time.Duration `yaml:"cb_timeout" env:"TM_EMAIL_CB_TIMEOUT" env-default:"30s"`
	CBFailures    uint32        `yaml:"cb_failures" env:"TM_EMAIL_CB_FAILURES" env-default:"5"`
}

// RateLimit — настройки ограничения запросов.
type RateLimit struct {
	RequestsPerMinute int `yaml:"requests_per_minute" env:"TM_RATE_LIMIT_RPM" env-default:"100"`
	Burst             int `yaml:"burst" env:"TM_RATE_LIMIT_BURST" env-default:"20"`
}

// TimeOuts — таймауты жизненного цикла приложения.
type TimeOuts struct {
	GracefulShutdown time.Duration `yaml:"graceful_shutdown" env:"TM_GRACEFUL_SHUTDOWN" env-default:"30s"`
}

// Load загружает конфигурацию из файла (если путь задан) и переменных окружения.
func Load(path string) (*Config, error) {
	cfg := &Config{}

	if path != "" {
		if err := cleanenv.ReadConfig(path, cfg); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}

		return cfg, nil
	}

	if err := cleanenv.ReadEnv(cfg); err != nil {
		return nil, fmt.Errorf("read env config: %w", err)
	}

	return cfg, nil
}
