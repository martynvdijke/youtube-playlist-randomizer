package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DefaultQuotaLimit   = 10000
	QuotaSafetyMarginPct = 5

	QuotaListPlaylists     = 1
	QuotaListPlaylistItems = 1
	QuotaCreatePlaylist    = 50
	QuotaInsertItem        = 50
)

func UsableLimit(limit int) int {
	margin := limit * QuotaSafetyMarginPct / 100
	if margin < 1 {
		margin = 1
	}
	usable := limit - margin
	if usable < 0 {
		usable = 0
	}
	return usable
}

type Store struct {
	db *sql.DB
}

// Severity levels in ascending order
const (
	SeverityDEBUG = "DEBUG"
	SeverityINFO  = "INFO"
	SeverityWARN  = "WARN"
	SeverityERROR = "ERROR"
)

func severityNumeric(s string) int {
	switch s {
	case SeverityERROR:
		return 3
	case SeverityWARN:
		return 2
	case SeverityINFO:
		return 1
	case SeverityDEBUG:
		return 0
	default:
		return 0
	}
}

type LogEntry struct {
	ID         int64  `json:"id"`
	Timestamp  string `json:"timestamp"`
	Severity   string `json:"severity"`
	Source     string `json:"source"`
	Message    string `json:"message"`
	Attributes string `json:"attributes,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type LogCount struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

type QuotaInfo struct {
	Date      string `json:"date"`
	Used      int    `json:"used"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
}

type Job struct {
	ID                string   `json:"id"`
	SourcePlaylistID  string   `json:"sourcePlaylistId"`
	SourcePlaylistIDs []string `json:"sourcePlaylistIds,omitempty"`
	SourceTitle       string   `json:"sourceTitle"`
	NewName           string   `json:"newName"`
	NewPlaylistID     string   `json:"newPlaylistId,omitempty"`
	Status            string   `json:"status"`
	TotalItems        int      `json:"totalItems"`
	InsertedItems     int      `json:"insertedItems"`
	Error             string   `json:"error,omitempty"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
	PausedAt          string   `json:"pausedAt,omitempty"`
	Archived          bool     `json:"archived"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS quota (
			date TEXT PRIMARY KEY,
			used INTEGER NOT NULL DEFAULT 0,
			quota_limit INTEGER NOT NULL DEFAULT 10000
		)`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			source_playlist_id TEXT NOT NULL,
			source_title TEXT NOT NULL DEFAULT '',
			new_name TEXT NOT NULL,
			new_playlist_id TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			total_items INTEGER DEFAULT 0,
			inserted_items INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS job_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			video_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			inserted INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (job_id) REFERENCES jobs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			severity TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			attributes TEXT DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// Backward-compat: ensure columns exist in databases created before they
	// were added to the initial schema. Silently ignore errors — the column
	// already exists in databases created with the current schema.
	for _, alter := range []string{
		"ALTER TABLE jobs ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN paused_at TEXT DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN source_playlist_ids TEXT DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN archived INTEGER NOT NULL DEFAULT 0",
	} {
		s.db.Exec(alter) //nolint:errcheck
	}
	return nil
}

func todayKey() string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Now().UTC().Format("2006-01-02")
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func (s *Store) GetQuota() (*QuotaInfo, error) {
	key := todayKey()
	row := s.db.QueryRow("SELECT used, quota_limit FROM quota WHERE date = ?", key)
	var used, limit int
	if err := row.Scan(&used, &limit); err == sql.ErrNoRows {
		return &QuotaInfo{Date: key, Used: 0, Limit: DefaultQuotaLimit, Remaining: UsableLimit(DefaultQuotaLimit)}, nil
	} else if err != nil {
		return nil, err
	}
	remaining := UsableLimit(limit) - used
	if remaining < 0 {
		remaining = 0
	}
	return &QuotaInfo{Date: key, Used: used, Limit: limit, Remaining: remaining}, nil
}

func (s *Store) AddQuota(units int) (*QuotaInfo, error) {
	key := todayKey()
	_, err := s.db.Exec(`INSERT INTO quota (date, used, quota_limit) VALUES (?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET used = used + ?`, key, units, DefaultQuotaLimit, units)
	if err != nil {
		return nil, err
	}
	return s.GetQuota()
}

func (s *Store) SetQuotaLimit(limit int) error {
	key := todayKey()
	_, err := s.db.Exec(`INSERT INTO quota (date, used, quota_limit) VALUES (?, 0, ?)
		ON CONFLICT(date) DO UPDATE SET quota_limit = ?`, key, limit, limit)
	return err
}

func (s *Store) EstimateQuotaNeeded(itemCount int) int {
	if itemCount == 0 {
		return 0
	}
	return QuotaCreatePlaylist + itemCount*QuotaInsertItem
}

func (s *Store) CreateJob(id, sourcePlaylistID, sourceTitle, newName string, extraPlaylistIDs ...string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	allIDs := sourcePlaylistID
	if len(extraPlaylistIDs) > 0 {
		allIDs = sourcePlaylistID + "," + joinIDs(extraPlaylistIDs)
	}
	_, err := s.db.Exec(`INSERT INTO jobs (id, source_playlist_id, source_title, new_name, source_playlist_ids, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`, id, sourcePlaylistID, sourceTitle, newName, allIDs, now, now)
	return err
}

func joinIDs(ids []string) string {
	var b []byte
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, id...)
	}
	return string(b)
}

func (s *Store) UpdateJobStatus(id, status string) error {
	_, err := s.db.Exec("UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?", status, nowRFC3339(), id)
	return err
}

func (s *Store) UpdateJobProgress(id string, insertedItems int, newPlaylistID string) error {
	_, err := s.db.Exec("UPDATE jobs SET inserted_items = ?, new_playlist_id = ?, updated_at = ? WHERE id = ?",
		insertedItems, newPlaylistID, nowRFC3339(), id)
	return err
}

func (s *Store) UpdateJobNewPlaylistID(id, newPlaylistID string) error {
	_, err := s.db.Exec("UPDATE jobs SET new_playlist_id = ?, updated_at = ? WHERE id = ?", newPlaylistID, nowRFC3339(), id)
	return err
}

func (s *Store) SetJobError(id, errMsg string) error {
	_, err := s.db.Exec("UPDATE jobs SET status = 'error', error = ?, updated_at = ? WHERE id = ?", errMsg, nowRFC3339(), id)
	return err
}

func (s *Store) SetJobPaused(id string) error {
	now := nowRFC3339()
	_, err := s.db.Exec("UPDATE jobs SET status = 'paused', paused_at = ?, updated_at = ? WHERE id = ?", now, now, id)
	return err
}

func (s *Store) SetJobDone(id string) error {
	_, err := s.db.Exec("UPDATE jobs SET status = 'done', updated_at = ? WHERE id = ?", nowRFC3339(), id)
	return err
}

func (s *Store) SetJobUndone(id string) error {
	_, err := s.db.Exec("UPDATE jobs SET status = 'undone', updated_at = ? WHERE id = ?", nowRFC3339(), id)
	return err
}

func (s *Store) DeleteJob(id string) error {
	_, err := s.db.Exec("DELETE FROM job_items WHERE job_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM jobs WHERE id = ?", id)
	return err
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *Store) SaveShuffledItems(jobID string, items []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO job_items (job_id, video_id, position, inserted) VALUES (?, ?, ?, 0)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, videoID := range items {
		if _, err := stmt.Exec(jobID, videoID, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetUninsertedItems(jobID string) ([]struct{ VideoID string; Position int }, error) {
	rows, err := s.db.Query("SELECT video_id, position FROM job_items WHERE job_id = ? AND inserted = 0 ORDER BY position", jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []struct{ VideoID string; Position int }
	for rows.Next() {
		var item struct{ VideoID string; Position int }
		if err := rows.Scan(&item.VideoID, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) MarkItemInserted(jobID string, position int) error {
	_, err := s.db.Exec("UPDATE job_items SET inserted = 1 WHERE job_id = ? AND position = ?", jobID, position)
	return err
}

func (s *Store) ResumeJob(id, newPlaylistID string) ([]struct{ VideoID string; Position int }, error) {
	items, err := s.GetUninsertedItems(id)
	if err != nil {
		return nil, err
	}
	if err := s.UpdateJobStatus(id, "inserting"); err != nil {
		return nil, err
	}
	if newPlaylistID != "" {
		if err := s.UpdateJobNewPlaylistID(id, newPlaylistID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func scanJob(scanner interface {
	Scan(dest ...interface{}) error
}) (*Job, error) {
	j := &Job{}
	var idsStr string
	var archived int
	err := scanner.Scan(&j.ID, &j.SourcePlaylistID, &j.SourceTitle, &j.NewName,
		&j.NewPlaylistID, &j.Status, &j.TotalItems, &j.InsertedItems,
		&j.Error, &j.CreatedAt, &j.UpdatedAt, &j.PausedAt, &idsStr, &archived)
	if err != nil {
		return nil, err
	}
	j.SourcePlaylistIDs = parseIDs(idsStr, j.SourcePlaylistID)
	j.Archived = archived == 1
	return j, nil
}

func parseIDs(idsStr, fallback string) []string {
	if idsStr == "" {
		if fallback != "" {
			return []string{fallback}
		}
		return nil
	}
	// Split by comma
	var ids []string
	start := 0
	for i := 0; i <= len(idsStr); i++ {
		if i == len(idsStr) || idsStr[i] == ',' {
			if i > start {
				ids = append(ids, idsStr[start:i])
			}
			start = i + 1
		}
	}
	return ids
}

const jobColumns = `id, source_playlist_id, source_title, new_name,
	COALESCE(new_playlist_id,''), status, total_items, inserted_items,
	COALESCE(error,''), created_at, COALESCE(updated_at,''), COALESCE(paused_at,''),
	COALESCE(source_playlist_ids,''), archived`

func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (s *Store) GetPendingJobs() ([]Job, error) {
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE status IN ('pending','paused','fetching','shuffling','inserting')
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (s *Store) GetLatestJob() (*Job, error) {
	row := s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs ORDER BY created_at DESC LIMIT 1`)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return j, nil
}

func (s *Store) GetDoneJobs() ([]Job, error) {
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE status = 'done' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (s *Store) GetAllJobs(limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE archived = 0 ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (s *Store) GetArchivedJobs(limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE archived = 1 ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (s *Store) ArchiveJob(id string) error {
	_, err := s.db.Exec("UPDATE jobs SET archived = 1, updated_at = ? WHERE id = ?", nowRFC3339(), id)
	return err
}

func (s *Store) InsertLog(entry LogEntry) error {
	now := nowRFC3339()
	_, err := s.db.Exec(`INSERT INTO logs (timestamp, severity, source, message, attributes, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.Timestamp, entry.Severity, entry.Source, entry.Message, entry.Attributes, now)
	return err
}

func (s *Store) GetLogs(minLevel string, sourceFilter string, limit, offset int) ([]LogEntry, error) {
	minN := severityNumeric(minLevel)

	query := `SELECT id, timestamp, severity, source, message, COALESCE(attributes,''), created_at FROM logs
		WHERE CASE severity WHEN 'ERROR' THEN 3 WHEN 'WARN' THEN 2 WHEN 'INFO' THEN 1 WHEN 'DEBUG' THEN 0 ELSE 0 END >= ?`
	args := []interface{}{minN}

	if sourceFilter != "" {
		query += ` AND source LIKE '%' || ? || '%'`
		args = append(args, sourceFilter)
	}

	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Severity, &e.Source, &e.Message, &e.Attributes, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) GetLogCounts() ([]LogCount, error) {
	rows, err := s.db.Query(`SELECT severity, COUNT(*) as cnt FROM logs GROUP BY severity ORDER BY
		CASE severity WHEN 'ERROR' THEN 0 WHEN 'WARN' THEN 1 WHEN 'INFO' THEN 2 WHEN 'DEBUG' THEN 3 ELSE 4 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []LogCount
	for rows.Next() {
		var c LogCount
		if err := rows.Scan(&c.Severity, &c.Count); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?`, key, value, value)
	return err
}
