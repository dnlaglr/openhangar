package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
)

var at = time.UnixMilli(1_790_000_000_000).UTC()

func openAt(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { s.Close() })

	return s
}

func seed(t *testing.T, s *Store) (domain.Project, domain.Workspace, domain.Chat) {
	t.Helper()
	ctx := t.Context()

	p := domain.Project{
		ID:            domain.NewID(),
		Name:          "hangar",
		RemoteURL:     "git@github.com:dnlaglr/openhangar.git",
		DefaultBranch: "main",
		SetupScript:   "pnpm install",
		RunScript:     "pnpm dev",
		SandboxImage:  "default",
		MergeMode:     domain.MergePR,
		CodeRemote:    domain.CodeRemoteProject,
		CreatedAt:     at,
	}

	w := domain.Workspace{
		ID: domain.NewID(), ProjectID: p.ID, Name: "Fix login", Slug: "fix-login",
		Status: domain.WorkspaceCreated, CreatedAt: at, UpdatedAt: at,
	}

	c := domain.Chat{
		ID: domain.NewID(), WorkspaceID: w.ID, Title: "Fix login",
		Agent:             domain.Agent{Provider: "claude-code", Model: "haiku", Effort: "low"},
		ProviderSessionID: "sess-1", CreatedAt: at,
	}

	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateWorkspace(ctx, w); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateChat(ctx, c); err != nil {
		t.Fatal(err)
	}

	return p, w, c
}

func TestMigrateAppliesOnlyPending(t *testing.T) {
	ctx := t.Context()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fsys := fstest.MapFS{"0001_a.sql": {Data: []byte("CREATE TABLE a (x INTEGER);")}}
	if err := migrate(ctx, conn, fsys); err != nil {
		t.Fatal(err)
	}

	fsys["0002_b.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (x INTEGER);")}
	if err := migrate(ctx, conn, fsys); err != nil {
		t.Fatalf("second run must skip 0001 and apply only 0002: %v", err)
	}

	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("user_version = %d, want 2", version)
	}
}

func TestReopenKeepsRowsAndMachineID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.db")
	s := openAt(t, path)
	p, w, c := seed(t, s)
	machineID := s.MachineID()
	s.Close()

	s = openAt(t, path)
	ctx := t.Context()
	if s.MachineID() != machineID || machineID == "" {
		t.Fatalf("machine id %q after reopen, want %q", s.MachineID(), machineID)
	}

	gotP, err := s.Project(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(gotP, p) {
		t.Fatalf("project = %+v, %v; want %+v", gotP, err, p)
	}

	gotW, err := s.Workspace(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(gotW, w) {
		t.Fatalf("workspace = %+v, %v; want %+v", gotW, err, w)
	}

	gotC, err := s.Chats(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(gotC, []domain.Chat{c}) {
		t.Fatalf("chats = %+v, %v; want [%+v]", gotC, err, c)
	}

	if _, err := s.Workspace(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing workspace err = %v, want ErrNotFound", err)
	}
}

func TestWorkspaceStatusIsCompareAndSet(t *testing.T) {
	s := openAt(t, filepath.Join(t.TempDir(), "engine.db"))
	_, w, _ := seed(t, s)
	ctx := t.Context()

	if err := s.SetWorkspaceStatus(ctx, w.ID, domain.WorkspaceCreated, domain.WorkspaceActive, at); err != nil {
		t.Fatal(err)
	}

	if err := s.SetWorkspaceStatus(ctx, w.ID, domain.WorkspaceCreated, domain.WorkspaceActive, at); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("stale from-status err = %v, want ErrInvalidTransition", err)
	}

	if err := s.SetWorkspaceStatus(ctx, w.ID, domain.WorkspaceActive, domain.WorkspaceArchived, at); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("active -> archived err = %v, want ErrInvalidTransition", err)
	}
}

func TestEventsAreAppendOnly(t *testing.T) {
	s := openAt(t, filepath.Join(t.TempDir(), "engine.db"))
	_, _, c := seed(t, s)
	ctx := t.Context()

	var want []domain.Event
	for _, id := range []string{"c", "b", "a"} {
		e := domain.Event{
			ID: id, ChatID: c.ID, Author: "user", MachineID: s.MachineID(),
			Type: "message", Payload: json.RawMessage(`{"text":"` + id + `"}`), CreatedAt: at,
		}
		if err := s.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
		want = append(want, e)
	}

	got, err := s.Events(ctx, c.ID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, %v; want append order %+v", got, err, want)
	}

	mustFail := func(query string) {
		t.Helper()
		if _, err := s.sql.ExecContext(ctx, query); err == nil {
			t.Fatalf("%q succeeded, events must be immutable", query)
		}
	}
	mustFail(`UPDATE events SET payload = '{}' WHERE id = 'a'`)
	mustFail(`DELETE FROM events WHERE id = 'a'`)
	if _, err := s.sql.ExecContext(ctx, `UPDATE events SET seq = 1 WHERE id = 'a'`); err != nil {
		t.Fatalf("assigning seq once must succeed: %v", err)
	}
	mustFail(`UPDATE events SET seq = 2 WHERE id = 'a'`)
}
