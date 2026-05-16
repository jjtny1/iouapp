package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// migrations are idempotent ALTER statements applied after the base schema so
// databases created before a column existed gain it on the next startup.
// A "duplicate column" error means the column is already present and is
// ignored; schema.sql carries the same columns for freshly created databases.
var migrations = []string{
	`ALTER TABLE bills ADD COLUMN service_charge_kind TEXT NOT NULL DEFAULT 'none'`,
	`ALTER TABLE bills ADD COLUMN service_charge_rate_bps INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE bills ADD COLUMN service_charge_cents INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE bills ADD COLUMN service_charge_headcount INTEGER NOT NULL DEFAULT 0`,
	// venmo_handle replaces the old USDC wallet_address as the host's payout
	// identity. The legacy wallet_address column is left in place on existing
	// databases — nothing reads it — so no drop migration is needed.
	`ALTER TABLE users ADD COLUMN venmo_handle TEXT`,
	// share_count lets a claim cover a fraction of an item: the claimer pays
	// 1/share_count of it, so a dish can be shared among several friends.
	`ALTER TABLE claims ADD COLUMN share_count INTEGER NOT NULL DEFAULT 1`,
}

type DB struct {
	*sql.DB
}

// Open connects to the SQLite database at path and applies the schema.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := conn.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("apply migration %q: %w", m, err)
		}
	}
	return &DB{conn}, nil
}
