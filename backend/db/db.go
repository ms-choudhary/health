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

func (d *DB) Close() error {
	return d.Conn.Close()
}
