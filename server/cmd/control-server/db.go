package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func openDatabase(ctx context.Context, databaseURL string) (*sql.DB, error) {
	dsn, err := mysqlDSN(databaseURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func mysqlDSN(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("DATABASE_URL is required")
	}
	if !strings.HasPrefix(raw, "mysql://") {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return "", fmt.Errorf("database name is required in DATABASE_URL")
	}
	query := u.Query()
	if query.Get("parseTime") == "" {
		query.Set("parseTime", "true")
	}
	if query.Get("loc") == "" {
		query.Set("loc", "UTC")
	}
	if query.Get("time_zone") == "" {
		query.Set("time_zone", "'+00:00'")
	}
	auth := user
	if pass != "" {
		auth += ":" + pass
	}
	return fmt.Sprintf("%s@tcp(%s)/%s?%s", auth, u.Host, database, query.Encode()), nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS policies (
			id VARCHAR(191) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			cluster_id VARCHAR(128) NOT NULL,
			namespace VARCHAR(128) NOT NULL DEFAULT '',
			controller_type VARCHAR(64) NOT NULL DEFAULT '',
			controller_name VARCHAR(128) NOT NULL DEFAULT '',
			pod_selector TEXT,
			pod_name VARCHAR(128) NOT NULL DEFAULT '',
			container_name VARCHAR(128) NOT NULL DEFAULT '',
			node_selector TEXT,
			log_type VARCHAR(64) NOT NULL,
			log_path TEXT,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			priority INT NOT NULL DEFAULT 100,
			current_revision INT NOT NULL DEFAULT 0,
			custom_fields JSON NULL,
			input_config JSON NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_policies_active (cluster_id, enabled, namespace, controller_type, controller_name, container_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS policy_revisions (
			policy_id VARCHAR(191) NOT NULL,
			revision INT NOT NULL,
			rendered_config LONGTEXT NOT NULL,
			checksum VARCHAR(128) NOT NULL,
			created_by VARCHAR(255) NOT NULL DEFAULT 'api',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (policy_id, revision),
			CONSTRAINT fk_policy_revisions_policy FOREIGN KEY (policy_id) REFERENCES policies(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS agents (
			id VARCHAR(191) PRIMARY KEY,
			cluster_id VARCHAR(128) NOT NULL,
			node_name VARCHAR(128) NOT NULL,
			pod_name VARCHAR(128) NOT NULL DEFAULT '',
			namespace VARCHAR(128) NOT NULL DEFAULT '',
			agent_version VARCHAR(128) NOT NULL DEFAULT '',
			filebeat_version VARCHAR(128) NOT NULL DEFAULT '',
			current_config_checksum VARCHAR(128) NOT NULL DEFAULT '',
			node_labels JSON NULL,
			capabilities JSON NULL,
			last_heartbeat_at TIMESTAMP NULL,
			last_apply_status VARCHAR(64) NOT NULL DEFAULT '',
			last_apply_message TEXT,
			last_apply_checksum VARCHAR(128) NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_agents_cluster (cluster_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS agent_apply_results (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			agent_id VARCHAR(191) NOT NULL,
			checksum VARCHAR(128) NOT NULL,
			status VARCHAR(64) NOT NULL,
			message TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_apply_results_agent (agent_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ensurePolicyInputConfigColumn(ctx, db); err != nil {
		return err
	}
	return ensureAgentCapabilitiesColumn(ctx, db)
}

func ensurePolicyInputConfigColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'policies' AND COLUMN_NAME = 'input_config'`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE policies ADD COLUMN input_config JSON NULL AFTER custom_fields`)
	return err
}

func ensureAgentCapabilitiesColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'agents' AND COLUMN_NAME = 'capabilities'`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE agents ADD COLUMN capabilities JSON NULL AFTER node_labels`)
	return err
}
