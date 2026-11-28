package worktree

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

const killGrace = 5 * time.Second

func (r Runtime) Exec(ctx context.Context, spec sandbox.Spec, c sandbox.Cmd) (sandbox.Process, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = r.Dir(spec.WorkspaceID)
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = killGrace

	p := &process{cmd: cmd}
	cmd.Stderr = &p.stderr
	cmd.Cancel = p.Kill

	var err error
	if p.stdin, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if p.stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return p, nil
}

func (Runtime) Isolated() bool { return false }

type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
	stderr bytes.Buffer
}

func (p *process) Stdin() io.WriteCloser { return p.stdin }

func (p *process) Stdout() io.Reader { return p.stdout }

// Kill stops the process group, so whatever the command started in the background goes with it.
// Children that move to their own process group or session escape; only a microVM contains those.
func (p *process) Kill() error {
	pgid := p.cmd.Process.Pid
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return err
	}
	time.AfterFunc(killGrace, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	return nil
}

func (p *process) Wait() error {
	if err := p.cmd.Wait(); err != nil {
		return fmt.Errorf("%s: %w: %s", p.cmd.Args[0], err, strings.TrimSpace(p.stderr.String()))
	}

	return nil
}
