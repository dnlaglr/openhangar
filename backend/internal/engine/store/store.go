package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	// Registers the "sqlite" database driver
	_ "modernc.org/sqlite"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

//go:embed migrations/*.sql
var embedded embed.FS

var migrations, _ = fs.Sub(embedded, "migrations")

type Store struct {
	sql       *sql.DB
	q         *db.Queries
	machineID string
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path + "?_txlock=immediate&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	s := &Store{sql: conn, q: db.New(conn)}
	if err := s.init(ctx); err != nil {
		conn.Close()

		return nil, err
	}

	return s, nil
}

func (s *Store) init(ctx context.Context) error {
	if err := migrate(ctx, s.sql, migrations); err != nil {
		return err
	}
	err := s.q.InsertMetaIfAbsent(ctx, db.InsertMetaIfAbsentParams{Key: "machine_id", Value: domain.NewID()})
	if err != nil {
		return err
	}
	s.machineID, err = s.q.GetMeta(ctx, "machine_id")

	return err
}

func (s *Store) Close() error {
	return s.sql.Close()
}

func (s *Store) MachineID() string {
	return s.machineID
}

func (s *Store) InTx(ctx context.Context, fn func(tx *Store) error) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Store{sql: s.sql, q: s.q.WithTx(tx), machineID: s.machineID}); err != nil {
		return errors.Join(err, tx.Rollback())
	}

	return tx.Commit()
}

func notFound(err error, what, id string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %s: %w", what, id, domain.ErrNotFound)
	}

	return err
}

func fromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
