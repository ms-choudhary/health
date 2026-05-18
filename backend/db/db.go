package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"health/db/queries"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type DB struct {
	Conn    *sql.DB
	Queries *queries.Queries
}

func Init(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("exec pragma foreign_keys: %w", err)
	}
	if err := ensureColumn(conn, "log_entries", "source_recipe_id", "INTEGER"); err != nil {
		return nil, fmt.Errorf("migrate log_entries.source_recipe_id: %w", err)
	}
	if err := ensureColumn(conn, "log_entries", "source_recipe_name", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrate log_entries.source_recipe_name: %w", err)
	}
	for _, stmt := range splitStatements(schemaSQL) {
		if _, err := conn.Exec(stmt); err != nil {
			return nil, fmt.Errorf("exec schema stmt: %w\n%s", err, stmt)
		}
	}
	return &DB{Conn: conn, Queries: queries.New(conn)}, nil
}

func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func ensureColumn(conn *sql.DB, table, column, definition string) error {
	var existing string
	err := conn.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
		table,
	).Scan(&existing)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (d *DB) Close() error {
	return d.Conn.Close()
}
