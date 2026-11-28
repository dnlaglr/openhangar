package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/chat"
	"github.com/dnlaglr/openhangar/backend/internal/engine/diagnostics"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/project"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider/claudecode"
	"github.com/dnlaglr/openhangar/backend/internal/engine/rpc"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox/worktree"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
	"github.com/dnlaglr/openhangar/backend/internal/engine/workspace"
	"github.com/dnlaglr/openhangar/backend/internal/shared/logging"
)

const shutdownGrace = 10 * time.Second

// handshake is the only line the engine writes to stdout; the launching client reads it to connect.
type handshake struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "engine:", err)
		os.Exit(1)
	}
}

// run serves until ctx is cancelled or stdin closes, which is how the engine notices its parent died.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	level := new(slog.LevelVar)
	flags := flag.NewFlagSet("engine", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", "", "directory for the database, repos and worktrees (default $XDG_DATA_HOME/hangar)")
	claudeBin := flags.String("claude-bin", "claude", "claude executable")
	flags.TextVar(level, "log-level", level, "debug, info, warn or error")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *dataDir == "" {
		dir, err := defaultDataDir()
		if err != nil {
			return err
		}
		*dataDir = dir
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return err
	}

	logDir, err := logging.Dir()
	if err != nil {
		return err
	}
	redactor := logging.SystemRedactor()
	logFile := logging.File(filepath.Join(logDir, "engine.log"))
	defer logFile.Close()
	rawFile := logging.File(filepath.Join(logDir, logging.RawDir, "claude.jsonl"))
	defer rawFile.Close()
	slog.SetDefault(logging.New(io.MultiWriter(logFile, stderr), level, redactor))

	st, err := store.Open(ctx, filepath.Join(*dataDir, "engine.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	repos := git.Repos{Root: filepath.Join(*dataDir, "repos")}
	rt := worktree.Runtime{Root: filepath.Join(*dataDir, "worktrees")}
	chats := &chat.Service{
		Store:   st,
		Repos:   repos,
		Runtime: rt,
		Providers: map[string]provider.Provider{
			claudecode.Name: claudecode.Provider{Runtime: rt, Bin: *claudeBin, Raw: logging.Raw(rawFile, level, redactor)},
		},
	}
	defer chats.Close()
	workspaces := workspace.Service{Store: st, Repos: repos, Runtime: rt}

	if err := workspaces.Reconcile(ctx); err != nil {
		return fmt.Errorf("reconcile workspaces: %w", err)
	}
	if err := chats.Recover(ctx); err != nil {
		return fmt.Errorf("recover chats: %w", err)
	}

	srv := &rpc.Server{
		Projects:    project.Service{Store: st, Repos: repos},
		Workspaces:  workspaces,
		Chats:       chats,
		LogLevel:    level,
		Diagnostics: diagnostics.Options{LogDir: logDir, Redactor: redactor, Runtime: "worktree"},
	}

	return serve(ctx, srv, stdin, stdout)
}

func serve(ctx context.Context, srv *rpc.Server, stdin io.Reader, stdout io.Writer) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		cancel()
	}()

	token := rand.Text()
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	httpSrv := &http.Server{
		Handler:           srv.Handler(token),
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	served := make(chan error, 1)
	go func() { served <- httpSrv.Serve(ln) }()

	if err := json.NewEncoder(stdout).Encode(handshake{Port: ln.Addr().(*net.TCPAddr).Port, Token: token}); err != nil {
		return err
	}
	slog.InfoContext(ctx, "engine listening", "addr", ln.Addr().String())

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancelShutdown()

	return httpSrv.Shutdown(shutdownCtx)
}

func defaultDataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "hangar"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "share", "hangar"), nil
}
