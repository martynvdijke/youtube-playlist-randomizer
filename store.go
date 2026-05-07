package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"golang.org/x/oauth2"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB(path string) error {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS oauth_token (
			id INTEGER PRIMARY KEY DEFAULT 1,
			token_data TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create oauth_token table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS quota_usage (
			date TEXT PRIMARY KEY,
			used INTEGER DEFAULT 0,
			daily_limit INTEGER DEFAULT 10000
		)
	`); err != nil {
		return fmt.Errorf("create quota_usage table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create settings table: %w", err)
	}

	return nil
}

func saveToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	_, err = db.Exec(
		"INSERT OR REPLACE INTO oauth_token (id, token_data, updated_at) VALUES (1, ?, CURRENT_TIMESTAMP)",
		string(data),
	)
	return err
}

func loadToken() (*oauth2.Token, error) {
	var data string
	err := db.QueryRow("SELECT token_data FROM oauth_token WHERE id = 1").Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &token, nil
}

func hasToken() bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM oauth_token WHERE id = 1").Scan(&count)
	return count > 0
}

func getTodayQuota() (int, int) {
	today := time.Now().Format("2006-01-02")

	var used int
	var dailyLimit int
	err := db.QueryRow("SELECT used, daily_limit FROM quota_usage WHERE date = ?", today).Scan(&used, &dailyLimit)
	if err == sql.ErrNoRows {
		return 0, getDailyLimit()
	}
	if err != nil {
		log.Printf("quota query error: %v", err)
		return 0, getDailyLimit()
	}
	return used, dailyLimit
}

func recordQuota(cost int) error {
	today := time.Now().Format("2006-01-02")
	limit := getDailyLimit()
	_, err := db.Exec(`
		INSERT INTO quota_usage (date, used, daily_limit)
		VALUES (?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET used = used + ?
	`, today, cost, limit, cost)
	return err
}

func remainingQuota() int {
	used, limit := getTodayQuota()
	if limit <= used {
		return 0
	}
	return limit - used
}

func quotaExpired() bool {
	return remainingQuota() <= 0
}

func getDailyLimit() int {
	var val string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'daily_quota_limit'").Scan(&val)
	if err != nil {
		return 10000
	}
	var limit int
	fmt.Sscanf(val, "%d", &limit)
	if limit <= 0 {
		return 10000
	}
	return limit
}

func setDailyLimit(limit int) {
	val := fmt.Sprintf("%d", limit)
	db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('daily_quota_limit', ?)", val)
}
