// Package conn содержит инициализацию пула соединений к MySQL.
package conn

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zyablitskiy/team-manager/internal/config"

	_ "github.com/go-sql-driver/mysql" // драйвер MySQL
)

// New создаёт *sql.DB с настроенным connection pooling и проверяет соединение.
func New(ctx context.Context, cfg *config.MySQL) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	// Connection pooling.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err = db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}
