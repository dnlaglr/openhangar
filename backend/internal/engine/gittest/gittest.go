package gittest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
)

func NewRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hangar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "README.md"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "init"},
	} {
		Must(t, dir, args...)
	}

	return dir
}

func Must(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git.Run(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
