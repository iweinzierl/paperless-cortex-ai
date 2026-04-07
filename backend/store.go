package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const configKey = "backend_config"

var errQueueItemNotPending = errors.New("queue item is not pending")
var errQueueItemNotFound = errors.New("queue item not found")
var errNoPendingQueueItems = errors.New("no pending queue items")

type Store struct {
	db *sql.DB
}

type Session struct {
	Token        string `json:"token"`
	Username     string `json:"username"`
	CreatedAtMS  int64  `json:"created_at_ms"`
	ExpiresAtMS  int64  `json:"expires_at_ms"`
	LastSeenAtMS int64  `json:"last_seen_at_ms"`
}

type QueueItem struct {
	ID                   int64  `json:"id"`
	DocumentID           *int64 `json:"document_id,omitempty"`
	DocumentTitle        string `json:"document_title"`
	Source               string `json:"source"`
	Trigger              string `json:"trigger"`
	Status               string `json:"status"`
	Payload              string `json:"payload,omitempty"`
	RequestedAtMS        int64  `json:"requested_at_ms"`
	StartedAtMS          *int64 `json:"started_at_ms,omitempty"`
	CompletedAtMS        *int64 `json:"completed_at_ms,omitempty"`
	Attempts             int    `json:"attempts"`
	LastError            string `json:"last_error,omitempty"`
	ResultSummary        string `json:"result_summary,omitempty"`
	UsedLLM              string `json:"used_llm,omitempty"`
	UsedVisionLLM        string `json:"used_vision_llm,omitempty"`
	ProcessingDurationMS *int64 `json:"processing_duration_ms,omitempty"`
}

type DashboardStats struct {
	QueuedCount           int64       `json:"queued_count"`
	AverageProcessingMS   float64     `json:"average_processing_time_ms"`
	ProcessingSuccessRate float64     `json:"processing_success_rate"`
	RecentRuns            []QueueItem `json:"recent_runs"`
}

func OpenStore(databasePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	store := &Store{db: database}
	if err := store.migrate(context.Background()); err != nil {
		database.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS config_entries (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			created_at_ms INTEGER NOT NULL,
			expires_at_ms INTEGER NOT NULL,
			last_seen_at_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS queue_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			document_id INTEGER,
			document_title TEXT NOT NULL,
			source TEXT NOT NULL,
			trigger_source TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL,
			requested_at_ms INTEGER NOT NULL,
			started_at_ms INTEGER,
			completed_at_ms INTEGER,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			result_summary TEXT NOT NULL DEFAULT '',
			used_llm TEXT NOT NULL DEFAULT '',
			used_vision_llm TEXT NOT NULL DEFAULT '',
			processing_duration_ms INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_items_status_requested_at ON queue_items(status, requested_at_ms)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_items_document_id_status ON queue_items(document_id, status)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite database: %w", err)
		}
	}

	if _, err := s.LoadConfig(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Store) LoadConfig(ctx context.Context) (BackendConfig, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config_entries WHERE key = ?`, configKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		cfg := DefaultBackendConfig()
		cfg.Normalize()
		if err := s.SaveConfig(ctx, cfg); err != nil {
			return BackendConfig{}, err
		}

		return cfg, nil
	}
	if err != nil {
		return BackendConfig{}, fmt.Errorf("load backend config: %w", err)
	}

	var cfg BackendConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return BackendConfig{}, fmt.Errorf("decode backend config: %w", err)
	}

	cfg.Normalize()
	return cfg, nil
}

func (s *Store) SaveConfig(ctx context.Context, cfg BackendConfig) error {
	cfg.Normalize()
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal backend config: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO config_entries(key, value, updated_at_ms)
		VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at_ms = excluded.updated_at_ms
	`, configKey, string(payload), nowMS()); err != nil {
		return fmt.Errorf("save backend config: %w", err)
	}

	return nil
}

func (s *Store) CreateSession(ctx context.Context, session Session) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(token, username, created_at_ms, expires_at_ms, last_seen_at_ms)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET
			username = excluded.username,
			created_at_ms = excluded.created_at_ms,
			expires_at_ms = excluded.expires_at_ms,
			last_seen_at_ms = excluded.last_seen_at_ms
	`, session.Token, session.Username, session.CreatedAtMS, session.ExpiresAtMS, session.LastSeenAtMS); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

func (s *Store) GetSession(ctx context.Context, token string) (*Session, error) {
	var session Session
	err := s.db.QueryRowContext(ctx, `
		SELECT token, username, created_at_ms, expires_at_ms, last_seen_at_ms
		FROM sessions
		WHERE token = ?
	`, token).Scan(&session.Token, &session.Username, &session.CreatedAtMS, &session.ExpiresAtMS, &session.LastSeenAtMS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	if session.ExpiresAtMS <= nowMS() {
		if _, deleteErr := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token); deleteErr != nil {
			return nil, fmt.Errorf("delete expired session: %w", deleteErr)
		}

		return nil, nil
	}

	return &session, nil
}

func (s *Store) TouchSession(ctx context.Context, token string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at_ms = ? WHERE token = ?`, nowMS(), token); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	return nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (s *Store) FindActiveQueueItemByDocumentID(ctx context.Context, documentID int64) (*QueueItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
		WHERE document_id = ? AND status IN ('pending', 'processing')
		ORDER BY requested_at_ms ASC
		LIMIT 1
	`, documentID)

	item, err := scanQueueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active queue item: %w", err)
	}

	return item, nil
}

func (s *Store) CreateQueueItem(ctx context.Context, documentID *int64, documentTitle string, source string, trigger string, payload string) (*QueueItem, error) {
	requestedAtMS := nowMS()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO queue_items(document_id, document_title, source, trigger_source, status, payload, requested_at_ms)
		VALUES(?, ?, ?, ?, 'pending', ?, ?)
	`, documentID, documentTitle, source, trigger, payload, requestedAtMS)
	if err != nil {
		return nil, fmt.Errorf("create queue item: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read queue item ID: %w", err)
	}

	return s.GetQueueItem(ctx, id)
}

func (s *Store) GetQueueItem(ctx context.Context, id int64) (*QueueItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
		WHERE id = ?
	`, id)

	item, err := scanQueueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errQueueItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get queue item: %w", err)
	}

	return item, nil
}

func (s *Store) ListQueueItems(ctx context.Context, status string, limit int) ([]QueueItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
	`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY requested_at_ms DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list queue items: %w", err)
	}
	defer rows.Close()

	items := make([]QueueItem, 0, limit)
	for rows.Next() {
		item, err := scanQueueItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan queue item: %w", err)
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue items: %w", err)
	}

	return items, nil
}

func (s *Store) ClaimNextPendingQueueItem(ctx context.Context) (*QueueItem, error) {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin queue transaction: %w", err)
	}
	defer transaction.Rollback()

	row := transaction.QueryRowContext(ctx, `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
		WHERE status = 'pending'
		ORDER BY requested_at_ms ASC
		LIMIT 1
	`)

	item, err := scanQueueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoPendingQueueItems
	}
	if err != nil {
		return nil, fmt.Errorf("select next queue item: %w", err)
	}

	startedAtMS := nowMS()
	result, err := transaction.ExecContext(ctx, `
		UPDATE queue_items
		SET status = 'processing', started_at_ms = ?, attempts = attempts + 1
		WHERE id = ? AND status = 'pending'
	`, startedAtMS, item.ID)
	if err != nil {
		return nil, fmt.Errorf("claim next queue item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read claim result: %w", err)
	}
	if rowsAffected == 0 {
		return nil, errNoPendingQueueItems
	}

	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit queue claim: %w", err)
	}

	item.Status = "processing"
	item.Attempts++
	item.StartedAtMS = &startedAtMS
	return item, nil
}

func (s *Store) ClaimQueueItemByID(ctx context.Context, id int64) (*QueueItem, error) {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin queue transaction: %w", err)
	}
	defer transaction.Rollback()

	row := transaction.QueryRowContext(ctx, `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
		WHERE id = ?
	`, id)

	item, err := scanQueueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errQueueItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select queue item: %w", err)
	}
	if item.Status != "pending" {
		return nil, errQueueItemNotPending
	}

	startedAtMS := nowMS()
	result, err := transaction.ExecContext(ctx, `
		UPDATE queue_items
		SET status = 'processing', started_at_ms = ?, attempts = attempts + 1
		WHERE id = ? AND status = 'pending'
	`, startedAtMS, item.ID)
	if err != nil {
		return nil, fmt.Errorf("claim queue item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read claim result: %w", err)
	}
	if rowsAffected == 0 {
		return nil, errQueueItemNotPending
	}

	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit queue claim: %w", err)
	}

	item.Status = "processing"
	item.Attempts++
	item.StartedAtMS = &startedAtMS
	return item, nil
}

func (s *Store) MarkQueueItemCompleted(ctx context.Context, id int64, summary string, usedLLM string, usedVisionLLM string, startedAtMS *int64) (*QueueItem, error) {
	completedAtMS := nowMS()
	var duration any
	if startedAtMS != nil && *startedAtMS > 0 {
		duration = completedAtMS - *startedAtMS
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE queue_items
		SET status = 'completed', completed_at_ms = ?, last_error = '', result_summary = ?,
		    used_llm = ?, used_vision_llm = ?, processing_duration_ms = ?
		WHERE id = ?
	`, completedAtMS, summary, usedLLM, usedVisionLLM, duration, id); err != nil {
		return nil, fmt.Errorf("mark queue item completed: %w", err)
	}

	return s.GetQueueItem(ctx, id)
}

func (s *Store) MarkQueueItemFailed(ctx context.Context, id int64, lastError string, usedLLM string, usedVisionLLM string, startedAtMS *int64) (*QueueItem, error) {
	completedAtMS := nowMS()
	var duration any
	if startedAtMS != nil && *startedAtMS > 0 {
		duration = completedAtMS - *startedAtMS
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE queue_items
		SET status = 'failed', completed_at_ms = ?, last_error = ?,
		    used_llm = ?, used_vision_llm = ?, processing_duration_ms = ?
		WHERE id = ?
	`, completedAtMS, lastError, usedLLM, usedVisionLLM, duration, id); err != nil {
		return nil, fmt.Errorf("mark queue item failed: %w", err)
	}

	return s.GetQueueItem(ctx, id)
}

func (s *Store) BuildDashboardStats(ctx context.Context, limit int) (DashboardStats, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	stats := DashboardStats{}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE status IN ('pending', 'processing')`).Scan(&stats.QueuedCount); err != nil {
		return DashboardStats{}, fmt.Errorf("count queued items: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(AVG(processing_duration_ms), 0) FROM queue_items WHERE status = 'completed'`).Scan(&stats.AverageProcessingMS); err != nil {
		return DashboardStats{}, fmt.Errorf("calculate average processing time: %w", err)
	}

	var completedCount int64
	var failedCount int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE status = 'completed'`).Scan(&completedCount); err != nil {
		return DashboardStats{}, fmt.Errorf("count completed items: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE status = 'failed'`).Scan(&failedCount); err != nil {
		return DashboardStats{}, fmt.Errorf("count failed items: %w", err)
	}

	totalProcessed := completedCount + failedCount
	if totalProcessed > 0 {
		stats.ProcessingSuccessRate = float64(completedCount) / float64(totalProcessed)
	}

	recentRuns, err := s.listRecentRuns(ctx, limit)
	if err != nil {
		return DashboardStats{}, err
	}
	stats.RecentRuns = recentRuns

	return stats, nil
}

func (s *Store) listRecentRuns(ctx context.Context, limit int) ([]QueueItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, document_id, document_title, source, trigger_source, status, payload,
		       requested_at_ms, started_at_ms, completed_at_ms, attempts, last_error,
		       result_summary, used_llm, used_vision_llm, processing_duration_ms
		FROM queue_items
		WHERE status IN ('completed', 'failed')
		ORDER BY completed_at_ms DESC, requested_at_ms DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent runs: %w", err)
	}
	defer rows.Close()

	items := make([]QueueItem, 0, limit)
	for rows.Next() {
		item, err := scanQueueItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recent run: %w", err)
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent runs: %w", err)
	}

	return items, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanQueueItem(row scanner) (*QueueItem, error) {
	var item QueueItem
	var documentID sql.NullInt64
	var startedAtMS sql.NullInt64
	var completedAtMS sql.NullInt64
	var durationMS sql.NullInt64

	err := row.Scan(
		&item.ID,
		&documentID,
		&item.DocumentTitle,
		&item.Source,
		&item.Trigger,
		&item.Status,
		&item.Payload,
		&item.RequestedAtMS,
		&startedAtMS,
		&completedAtMS,
		&item.Attempts,
		&item.LastError,
		&item.ResultSummary,
		&item.UsedLLM,
		&item.UsedVisionLLM,
		&durationMS,
	)
	if err != nil {
		return nil, err
	}

	if documentID.Valid {
		item.DocumentID = &documentID.Int64
	}
	if startedAtMS.Valid {
		item.StartedAtMS = &startedAtMS.Int64
	}
	if completedAtMS.Valid {
		item.CompletedAtMS = &completedAtMS.Int64
	}
	if durationMS.Valid {
		item.ProcessingDurationMS = &durationMS.Int64
	}

	return &item, nil
}
