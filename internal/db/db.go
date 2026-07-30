// Package db opens CraftDeck's SQLite database (WAL mode, pure-Go driver so
// the daemon stays a single static binary per NFR-9) and applies embedded
// migrations on startup.
package db

import (
	"database/sql"
	"fmt"
	"sort"

	"craftdeck/internal/db/migrations"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	// busy_timeout: WAL lets readers and one writer coexist, but two writers
	// still can't overlap -- and without a timeout SQLite fails such a
	// collision instantly with "database is locked" rather than waiting.
	// craftdeckd has several independent writers (HTTP handlers, the DDNS
	// reconciler, the daily log-retention sweep, startup reconciliation), so
	// brief overlaps are normal; 5s of waiting turns them into a slight delay
	// instead of a spurious error surfaced to the operator.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var already string
		err := db.QueryRow(`SELECT filename FROM schema_migrations WHERE filename = ?`, name).Scan(&already)
		if err == nil {
			continue // already applied
		}
		if err != sql.ErrNoRows {
			return err
		}

		sqlBytes, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (filename) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
