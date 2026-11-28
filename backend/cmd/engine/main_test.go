package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/dnlaglr/openhangar/backend/gen/hangar/v1"
	"github.com/dnlaglr/openhangar/backend/gen/hangar/v1/hangarv1connect"
)

func TestHandshakeServesH2CUntilStdinCloses(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	stdin, closeStdin := io.Pipe()
	stdoutR, stdout := io.Pipe()

	done := make(chan error, 1)
	go func() { done <- run(t.Context(), []string{"-data-dir", t.TempDir()}, stdin, stdout, io.Discard) }()

	line, err := bufio.NewReader(stdoutR).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, stdoutR) }()

	var hs handshake
	if err := json.Unmarshal(line, &hs); err != nil || hs.Port == 0 || len(hs.Token) < 26 {
		t.Fatalf("handshake %q: %+v, %v", line, hs, err)
	}

	h2c := &http.Client{Transport: &http.Transport{Protocols: h2cOnly()}}
	projects := hangarv1connect.NewProjectServiceClient(h2c, fmt.Sprintf("http://127.0.0.1:%d", hs.Port))
	req := connect.NewRequest(&v1.ListProjectsRequest{})
	req.Header().Set("Authorization", "Bearer "+hs.Token)
	res, err := projects.ListProjects(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Msg.GetProjects()) != 0 {
		t.Fatalf("projects = %v, want none in a fresh data dir", res.Msg.GetProjects())
	}

	closeStdin.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run = %v, want clean shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("engine kept running after stdin closed")
	}
}

func h2cOnly() *http.Protocols {
	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)

	return p
}
