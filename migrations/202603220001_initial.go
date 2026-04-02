package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202603220001 初始化所有表
func migration202603220001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603220001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`CREATE TABLE IF NOT EXISTS assets (
					id             INTEGER PRIMARY KEY AUTOINCREMENT,
					name           VARCHAR(255) NOT NULL,
					type           VARCHAR(50)  NOT NULL,
					group_id       INTEGER DEFAULT 0,
					icon           VARCHAR(100),
					tags           TEXT,
					description    TEXT,
					config         TEXT,
					command_policy TEXT,
					sort_order     INTEGER DEFAULT 0,
					status         INTEGER DEFAULT 1,
					createtime     INTEGER,
					updatetime     INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_assets_type ON assets (type)`,
				`CREATE INDEX IF NOT EXISTS idx_assets_group_id ON assets (group_id)`,

				`CREATE TABLE IF NOT EXISTS groups (
					id             INTEGER PRIMARY KEY AUTOINCREMENT,
					name           VARCHAR(255) NOT NULL,
					parent_id      INTEGER DEFAULT 0,
					icon           VARCHAR(100),
					description    TEXT,
					command_policy TEXT,
					query_policy   TEXT,
					redis_policy   TEXT,
					sort_order     INTEGER DEFAULT 0,
					createtime     INTEGER,
					updatetime     INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_groups_parent_id ON groups (parent_id)`,

				`CREATE TABLE IF NOT EXISTS conversations (
					id            INTEGER PRIMARY KEY AUTOINCREMENT,
					title         VARCHAR(255),
					provider_type VARCHAR(50) NOT NULL,
					model         VARCHAR(100),
					session_data  TEXT,
					work_dir      VARCHAR(500),
					status        INTEGER DEFAULT 1,
					createtime    INTEGER,
					updatetime    INTEGER
				)`,

				`CREATE TABLE IF NOT EXISTS conversation_messages (
					id              INTEGER PRIMARY KEY AUTOINCREMENT,
					conversation_id INTEGER NOT NULL,
					role            VARCHAR(20) NOT NULL,
					content         TEXT,
					tool_calls      TEXT,
					tool_call_id    VARCHAR(100),
					blocks          TEXT,
					sort_order      INTEGER DEFAULT 0,
					createtime      INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON conversation_messages (conversation_id)`,

				`CREATE TABLE IF NOT EXISTS audit_logs (
					id               INTEGER PRIMARY KEY AUTOINCREMENT,
					source           VARCHAR(20)  NOT NULL,
					tool_name        VARCHAR(100) NOT NULL,
					asset_id         INTEGER DEFAULT 0,
					asset_name       VARCHAR(255),
					command          TEXT,
					request          TEXT,
					result           TEXT,
					error            TEXT,
					success          INTEGER DEFAULT 1,
					conversation_id  INTEGER DEFAULT 0,
					grant_session_id VARCHAR(36),
					session_id       VARCHAR(64),
					decision         VARCHAR(10),
					decision_source  VARCHAR(30),
					matched_pattern  VARCHAR(500),
					createtime       INTEGER NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_source ON audit_logs (source)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_asset_id ON audit_logs (asset_id)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_conversation_id ON audit_logs (conversation_id)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_session_id ON audit_logs (session_id)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_createtime ON audit_logs (createtime)`,

				`CREATE TABLE IF NOT EXISTS grant_sessions (
					id          VARCHAR(36) PRIMARY KEY,
					description TEXT,
					status      INTEGER NOT NULL DEFAULT 1,
					createtime  INTEGER NOT NULL,
					updatetime  INTEGER
				)`,

				`CREATE TABLE IF NOT EXISTS grant_items (
					id               INTEGER PRIMARY KEY AUTOINCREMENT,
					grant_session_id VARCHAR(36)  NOT NULL,
					item_index       INTEGER      NOT NULL,
					tool_name        VARCHAR(100) NOT NULL,
					asset_id         INTEGER DEFAULT 0,
					asset_name       VARCHAR(255),
					group_id         INTEGER DEFAULT 0,
					group_name       VARCHAR(255),
					command          TEXT,
					detail           TEXT,
					createtime       INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_grant_items_session_id ON grant_items (grant_session_id)`,

				`CREATE TABLE IF NOT EXISTS forward_configs (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					name       VARCHAR(255) NOT NULL,
					asset_id   INTEGER NOT NULL,
					createtime INTEGER,
					updatetime INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_forward_configs_asset_id ON forward_configs (asset_id)`,

				`CREATE TABLE IF NOT EXISTS forward_rules (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					config_id   INTEGER      NOT NULL,
					type        VARCHAR(20)  NOT NULL,
					local_host  VARCHAR(255) NOT NULL,
					local_port  INTEGER      NOT NULL,
					remote_host VARCHAR(255) NOT NULL,
					remote_port INTEGER      NOT NULL,
					createtime  INTEGER,
					updatetime  INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_forward_rules_config_id ON forward_rules (config_id)`,

				`CREATE TABLE IF NOT EXISTS credentials (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					name        VARCHAR(255) NOT NULL,
					type        VARCHAR(50)  NOT NULL,
					username    VARCHAR(255),
					password    TEXT,
					private_key TEXT,
					public_key  TEXT,
					key_type    VARCHAR(50),
					key_size    INTEGER,
					fingerprint VARCHAR(255),
					comment     VARCHAR(255),
					description TEXT,
					createtime  INTEGER,
					updatetime  INTEGER
				)`,

				`CREATE TABLE IF NOT EXISTS host_keys (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					host        VARCHAR(255),
					port        INTEGER,
					key_type    VARCHAR(50)  NOT NULL,
					public_key  TEXT         NOT NULL,
					fingerprint VARCHAR(255) NOT NULL,
					first_seen  INTEGER,
					last_seen   INTEGER
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_host_port_type ON host_keys (host, port, key_type)`,

				`CREATE TABLE IF NOT EXISTS policy_groups (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					name        VARCHAR(255) NOT NULL,
					description TEXT,
					policy_type VARCHAR(50) NOT NULL,
					policy      TEXT        NOT NULL,
					createtime  INTEGER,
					updatetime  INTEGER
				)`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
