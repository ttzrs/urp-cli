package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/joss/urp/internal/opencode/domain"
)

type Storage struct {
	db   *sql.DB
	path string
}

// Verify Storage implements domain.Store
var _ domain.Store = (*Storage)(nil)

func New(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "opencode.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Storage{db: db, path: dbPath}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		directory TEXT NOT NULL,
		parent_id TEXT,
		title TEXT NOT NULL,
		version TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		summary_json TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		parts_json TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(session_id, timestamp);

	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS usage (
		session_id TEXT PRIMARY KEY,
		provider_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		cache_read INTEGER NOT NULL DEFAULT 0,
		cache_write INTEGER NOT NULL DEFAULT 0,
		input_cost REAL NOT NULL DEFAULT 0,
		output_cost REAL NOT NULL DEFAULT 0,
		total_cost REAL NOT NULL DEFAULT 0,
		message_count INTEGER NOT NULL DEFAULT 0,
		tool_calls INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_usage_updated ON usage(updated_at DESC);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// Session operations

func (s *Storage) CreateSession(ctx context.Context, sess *domain.Session) error {
	summaryJSON, _ := json.Marshal(sess.Summary)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, directory, parent_id, title, version, created_at, updated_at, summary_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sess.ID, sess.ProjectID, sess.Directory, sess.ParentID, sess.Title, sess.Version, sess.CreatedAt, sess.UpdatedAt, summaryJSON)
	return err
}

func (s *Storage) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	var sess domain.Session
	var summaryJSON sql.NullString
	var parentID sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, directory, parent_id, title, version, created_at, updated_at, summary_json
		FROM sessions WHERE id = ?
	`, id).Scan(&sess.ID, &sess.ProjectID, &sess.Directory, &parentID, &sess.Title, &sess.Version, &sess.CreatedAt, &sess.UpdatedAt, &summaryJSON)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		sess.ParentID = parentID.String
	}
	if summaryJSON.Valid {
		json.Unmarshal([]byte(summaryJSON.String), &sess.Summary)
	}
	return &sess, nil
}

func (s *Storage) ListSessions(ctx context.Context, projectID string, limit int) ([]*domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, directory, parent_id, title, version, created_at, updated_at, summary_json
		FROM sessions WHERE project_id = ? ORDER BY updated_at DESC LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		var sess domain.Session
		var summaryJSON sql.NullString
		var parentID sql.NullString

		if err := rows.Scan(&sess.ID, &sess.ProjectID, &sess.Directory, &parentID, &sess.Title, &sess.Version, &sess.CreatedAt, &sess.UpdatedAt, &summaryJSON); err != nil {
			return nil, err
		}

		if parentID.Valid {
			sess.ParentID = parentID.String
		}
		if summaryJSON.Valid {
			json.Unmarshal([]byte(summaryJSON.String), &sess.Summary)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}

func (s *Storage) UpdateSession(ctx context.Context, sess *domain.Session) error {
	summaryJSON, _ := json.Marshal(sess.Summary)
	sess.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET title = ?, updated_at = ?, summary_json = ? WHERE id = ?
	`, sess.Title, sess.UpdatedAt, summaryJSON, sess.ID)
	return err
}

func (s *Storage) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// Message operations

func (s *Storage) CreateMessage(ctx context.Context, msg *domain.Message) error {
	partsJSON, err := domain.MarshalParts(msg.Parts)
	if err != nil {
		return fmt.Errorf("marshal parts: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, role, parts_json, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`, msg.ID, msg.SessionID, msg.Role, partsJSON, msg.Timestamp)
	return err
}

func (s *Storage) GetMessages(ctx context.Context, sessionID string) ([]*domain.Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, role, parts_json, timestamp
		FROM messages WHERE session_id = ? ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var msg domain.Message
		var partsJSON string

		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &partsJSON, &msg.Timestamp); err != nil {
			return nil, err
		}

		parts, err := domain.UnmarshalParts([]byte(partsJSON))
		if err != nil {
			return nil, fmt.Errorf("unmarshal parts: %w", err)
		}
		msg.Parts = parts
		messages = append(messages, &msg)
	}
	return messages, nil
}

func (s *Storage) UpdateMessage(ctx context.Context, msg *domain.Message) error {
	partsJSON, err := domain.MarshalParts(msg.Parts)
	if err != nil {
		return fmt.Errorf("marshal parts: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE messages SET parts_json = ? WHERE id = ?
	`, partsJSON, msg.ID)
	return err
}

func (s *Storage) DeleteMessage(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	return err
}

// Config operations

func (s *Storage) GetConfig(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (s *Storage) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// Usage operations

func (s *Storage) GetUsage(ctx context.Context, sessionID string) (*domain.SessionUsage, error) {
	var u domain.SessionUsage
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, provider_id, model_id, input_tokens, output_tokens,
			   cache_read, cache_write, input_cost, output_cost, total_cost,
			   message_count, tool_calls, updated_at
		FROM usage WHERE session_id = ?
	`, sessionID).Scan(
		&u.SessionID, &u.ProviderID, &u.ModelID,
		&u.Usage.InputTokens, &u.Usage.OutputTokens,
		&u.Usage.CacheRead, &u.Usage.CacheWrite,
		&u.Usage.InputCost, &u.Usage.OutputCost, &u.Usage.TotalCost,
		&u.MessageCount, &u.ToolCalls, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (s *Storage) UpdateUsage(ctx context.Context, u *domain.SessionUsage) error {
	u.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO usage (session_id, provider_id, model_id, input_tokens, output_tokens,
						   cache_read, cache_write, input_cost, output_cost, total_cost,
						   message_count, tool_calls, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			cache_read = cache_read + excluded.cache_read,
			cache_write = cache_write + excluded.cache_write,
			input_cost = input_cost + excluded.input_cost,
			output_cost = output_cost + excluded.output_cost,
			total_cost = total_cost + excluded.total_cost,
			message_count = message_count + excluded.message_count,
			tool_calls = tool_calls + excluded.tool_calls,
			updated_at = excluded.updated_at
	`, u.SessionID, u.ProviderID, u.ModelID,
		u.Usage.InputTokens, u.Usage.OutputTokens,
		u.Usage.CacheRead, u.Usage.CacheWrite,
		u.Usage.InputCost, u.Usage.OutputCost, u.Usage.TotalCost,
		u.MessageCount, u.ToolCalls, u.UpdatedAt,
	)
	return err
}

func (s *Storage) GetTotalUsage(ctx context.Context) (*domain.Usage, error) {
	var u domain.Usage
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			   COALESCE(SUM(cache_read), 0), COALESCE(SUM(cache_write), 0),
			   COALESCE(SUM(input_cost), 0), COALESCE(SUM(output_cost), 0),
			   COALESCE(SUM(total_cost), 0)
		FROM usage
	`).Scan(&u.InputTokens, &u.OutputTokens, &u.CacheRead, &u.CacheWrite,
		&u.InputCost, &u.OutputCost, &u.TotalCost)
	return &u, err
}
