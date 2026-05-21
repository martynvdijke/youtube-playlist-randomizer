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

type QuotaInfo struct {
	Date      string `json:"date"`
	Used      int    `json:"used"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
}

type Job struct {
	ID               string  `json:"id"`
	SourcePlaylistID string  `json:"sourcePlaylistId"`
	SourceTitle      string  `json:"sourceTitle"`
	NewName          string  `json:"newName"`
	NewPlaylistID    string  `json:"newPlaylistId,omitempty"`
	Status           string  `json:"status"`
	TotalItems       int     `json:"totalItems"`
	InsertedItems    int     `json:"insertedItems"`
	Error            string  `json:"error,omitempty"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
	PausedAt         string  `json:"pausedAt,omitempty"`
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
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// Add updated_at column if missing (existing databases)
	s.db.Exec("ALTER TABLE jobs ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''")
	s.db.Exec("ALTER TABLE jobs ADD COLUMN paused_at TEXT DEFAULT ''")
	return nil
}

func todayKey() string {
	return time.Now().UTC().Format("2006-01-02")
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

func (s *Store) CreateJob(id, sourcePlaylistID, sourceTitle, newName string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO jobs (id, source_playlist_id, source_title, new_name, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'pending', ?, ?)`, id, sourcePlaylistID, sourceTitle, newName, now, now)
	return err
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

func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT id, source_playlist_id, source_title, new_name,
		COALESCE(new_playlist_id,''), status, total_items, inserted_items,
		COALESCE(error,''), created_at, COALESCE(updated_at,''), COALESCE(paused_at,'') FROM jobs WHERE id = ?`, id)
	j := &Job{}
	if err := row.Scan(&j.ID, &j.SourcePlaylistID, &j.SourceTitle, &j.NewName,
		&j.NewPlaylistID, &j.Status, &j.TotalItems, &j.InsertedItems,
		&j.Error, &j.CreatedAt, &j.UpdatedAt, &j.PausedAt); err != nil {
		return nil, err
	}
	return j, nil
}

func (s *Store) GetPendingJobs() ([]Job, error) {
	rows, err := s.db.Query(`SELECT id, source_playlist_id, source_title, new_name,
		COALESCE(new_playlist_id,''), status, total_items, inserted_items,
		COALESCE(error,''), created_at, COALESCE(updated_at,''), COALESCE(paused_at,'') FROM jobs WHERE status IN ('pending','paused','fetching','shuffling','inserting')
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.SourcePlaylistID, &j.SourceTitle, &j.NewName,
			&j.NewPlaylistID, &j.Status, &j.TotalItems, &j.InsertedItems,
			&j.Error, &j.CreatedAt, &j.UpdatedAt, &j.PausedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) GetLatestJob() (*Job, error) {
	row := s.db.QueryRow(`SELECT id, source_playlist_id, source_title, new_name,
		COALESCE(new_playlist_id,''), status, total_items, inserted_items,
		COALESCE(error,''), created_at, COALESCE(updated_at,''), COALESCE(paused_at,'') FROM jobs ORDER BY created_at DESC LIMIT 1`)
	j := &Job{}
	if err := row.Scan(&j.ID, &j.SourcePlaylistID, &j.SourceTitle, &j.NewName,
		&j.NewPlaylistID, &j.Status, &j.TotalItems, &j.InsertedItems,
		&j.Error, &j.CreatedAt, &j.UpdatedAt, &j.PausedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return j, nil
}
