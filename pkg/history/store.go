// Package history keeps the runs a Mini App user chose to save.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	// CGO-free SQLite: the Docker image is built with CGO_ENABLED=0.
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound   = errors.New("run not found")
	ErrInvalidRun = errors.New("distance and time must be greater than zero")
)

// Run is one saved calculation. Pace is not stored: distance and time fully
// determine it, the same way the shareable URL does.
type Run struct {
	ID       int64
	Distance int           // meters
	Time     time.Duration // finish time, whole seconds
	SavedAt  time.Time
}

type Store struct {
	db *sql.DB
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS saved_runs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL,
		distance_m INTEGER NOT NULL CHECK (distance_m > 0),
		time_s     INTEGER NOT NULL CHECK (time_s > 0),
		saved_at   INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS saved_runs_by_user ON saved_runs (user_id, saved_at DESC, id DESC)`,
}

// Open opens the database file at path, creating it and its schema if needed.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}

	// SQLite has a single write lock anyway; one connection turns
	// "database is locked" under concurrent requests into plain queueing.
	db.SetMaxOpenConns(1)

	for _, statement := range schema {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply history schema: %w", err)
		}
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Save stores a run for userID and returns it with its new id.
func (s *Store) Save(ctx context.Context, userID int64, distance int, finish time.Duration, now time.Time) (Run, error) {
	seconds := int64(finish / time.Second)
	if distance <= 0 || seconds <= 0 {
		return Run{}, ErrInvalidRun
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO saved_runs (user_id, distance_m, time_s, saved_at) VALUES (?, ?, ?, ?)`,
		userID, distance, seconds, now.Unix())
	if err != nil {
		return Run{}, fmt.Errorf("save run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Run{}, fmt.Errorf("save run: %w", err)
	}

	return Run{
		ID:       id,
		Distance: distance,
		Time:     time.Duration(seconds) * time.Second,
		SavedAt:  time.Unix(now.Unix(), 0),
	}, nil
}

// List returns up to limit of userID's runs, newest first.
func (s *Store) List(ctx context.Context, userID int64, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, distance_m, time_s, saved_at FROM saved_runs
		 WHERE user_id = ? ORDER BY saved_at DESC, id DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := []Run{}
	for rows.Next() {
		var run Run
		var seconds, savedAt int64
		if err := rows.Scan(&run.ID, &run.Distance, &seconds, &savedAt); err != nil {
			return nil, fmt.Errorf("list runs: %w", err)
		}

		run.Time = time.Duration(seconds) * time.Second
		run.SavedAt = time.Unix(savedAt, 0)
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	return runs, nil
}

// Delete removes one of userID's runs. Someone else's run is reported as not
// found rather than forbidden, so ids cannot be probed.
func (s *Store) Delete(ctx context.Context, userID int64, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM saved_runs WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete run: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete run: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}
