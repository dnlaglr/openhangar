package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/dnlaglr/openhangar/backend/internal/shared/logging"
)

func TestWriteRedactsAndSkipsRaw(t *testing.T) {
	secret := "ghp_" + strings.Repeat("s", 36)
	logDir := t.TempDir()
	writeFile(t, filepath.Join(logDir, "engine.log"), `{"msg":"clone failed","err":"`+secret+` in /home/alice/repos"}`+"\n")
	writeFile(t, filepath.Join(logDir, logging.RawDir, "provider.log"), "PROMPT-CONTENT\n")

	outside := filepath.Join(t.TempDir(), "outside.log")
	writeFile(t, outside, "OUTSIDE-CONTENT\n")
	if err := os.Symlink(outside, filepath.Join(logDir, "linked.log")); err != nil {
		t.Fatal(err)
	}

	files := bundle(t, Options{
		LogDir:     logDir,
		Redactor:   logging.NewRedactor("/home/alice", ""),
		AppVersion: "1.2.3",
		Runtime:    "worktree",
	})

	if names, want := slices.Sorted(maps.Keys(files)), []string{"logs/engine.log", "system.json"}; !slices.Equal(names, want) {
		t.Fatalf("entries %v, want %v", names, want)
	}

	for name, data := range files {
		for _, leak := range []string{secret, "/home/alice", "PROMPT-CONTENT", "OUTSIDE-CONTENT"} {
			if strings.Contains(data, leak) {
				t.Errorf("%s leaks %q", name, leak)
			}
		}
	}

	if !strings.Contains(files["logs/engine.log"], "in ~/repos") {
		t.Errorf("engine.log not re-redacted as expected: %s", files["logs/engine.log"])
	}

	var sys System
	if err := json.Unmarshal([]byte(files["system.json"]), &sys); err != nil {
		t.Fatal(err)
	}
	if sys.AppVersion != "1.2.3" || sys.Runtime != "worktree" || sys.Arch != runtime.GOARCH || sys.KVM == "" {
		t.Errorf("unexpected system.json: %+v", sys)
	}
}

func TestWriteWithoutLogDir(t *testing.T) {
	files := bundle(t, Options{
		LogDir:   filepath.Join(t.TempDir(), "missing"),
		Redactor: logging.NewRedactor("", ""),
	})

	if _, ok := files["system.json"]; !ok || len(files) != 1 {
		t.Fatalf("entries %v, want only system.json", files)
	}
}

func bundle(t *testing.T, opts Options) map[string]string {
	t.Helper()

	var buf bytes.Buffer
	if err := Write(&buf, opts); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(data)
	}

	return files
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
