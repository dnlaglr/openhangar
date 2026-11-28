package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

func migrate(ctx context.Context, conn *sql.DB, fsys fs.FS) error {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}

	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}

	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		if err := apply(ctx, conn, fsys, file, version); err != nil {
			return fmt.Errorf("migration %s: %w", file, err)
		}
	}

	return nil
}

func migrationVersion(file string) (int, error) {
	prefix, _, _ := strings.Cut(file, "_")
	return strconv.Atoi(prefix)
}

func apply(ctx context.Context, conn *sql.DB, fsys fs.FS, file string, version int) error {
	body, err := fs.ReadFile(fsys, file)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return errors.Join(err, tx.Rollback())
	}

	return tx.Commit()
}
