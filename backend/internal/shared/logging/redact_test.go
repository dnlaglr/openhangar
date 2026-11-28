package logging

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("a1", 18)
	jwt := "eyJ" + strings.Repeat("h", 10) + ".eyJ" + strings.Repeat("p", 10) + "." + strings.Repeat("s", 12)

	cases := []struct{ name, in, want string }{
		{"url with credentials", "clone https://x-access-token:" + ghp + "@github.com/acme/repo.git failed", "clone https://<redacted> failed"},
		{"url inside json", `{"remote":"ssh://git@github.com/acme/repo"}`, `{"remote":"ssh://<redacted>"}`},
		{"url query", "GET https://tower.acme.dev/v1?token=abc", "GET https://<redacted>"},
		{"scp remote", "fetch git@github.com:acme/repo.git", "fetch <redacted>"},
		{"secret json key", `{"api_key":"abc123","level":"INFO"}`, `{"api_key":"<redacted>","level":"INFO"}`},
		{"secret json key escaped quote", `{"session_token":"a\"b","x":1}`, `{"session_token":"<redacted>","x":1}`},
		{"secret key=value", "password=hunter2 user=bob", "password=<redacted> user=bob"},
		{"authorization header", "Authorization: Basic dXNlcjpwYXNz", "Authorization: Basic <redacted>"},
		{"bearer", "sent bearer abcdefgh.ijk", "sent bearer <redacted>"},
		{"anthropic api key", "key sk-ant-api03-" + strings.Repeat("x", 40), "key <redacted>"},
		{"anthropic oauth token", "sk-ant-oat01-" + strings.Repeat("y", 40) + " expired", "<redacted> expired"},
		{"github token", "token " + ghp, "token <redacted>"},
		{"github pat", "github_pat_" + strings.Repeat("Z", 30), "<redacted>"},
		{"aws key", "AKIA" + strings.Repeat("Q", 16), "<redacted>"},
		{"jwt", "jwt " + jwt, "jwt <redacted>"},
		{"email", "author dev@acme.io committed", "author <redacted> committed"},
		{"wsl windows home", "open /mnt/c/Users/Alice/src/app", "open <winhome>/src/app"},
		{"json windows home", `{"p":"C:\\Users\\Alice\\src"}`, `{"p":"<winhome>\\src"}`},
		{"home path", "~ /home/alice/.cache/hangar/logs/engine.log", "~ ~/.cache/hangar/logs/engine.log"},
		{"home at end of json string", `{"dir":"/home/alice"}`, `{"dir":"~"}`},
		{"hostname", "dial from devbox-01 failed", "dial from <host> failed"},
		{"branch in git args", "git branch -D -- hangar/fix-acme-billing", "git branch -D -- hangar/<redacted>"},
		{"branch ref", "refs/heads/hangar/fix-acme-billing", "refs/heads/hangar/<redacted>"},
		{"keeps ids and counts", `{"workspace_id":"0193b6a4-7c1e-7d2a-9f00-5e1d2c3b4a59","input_tokens":1200}`, `{"workspace_id":"0193b6a4-7c1e-7d2a-9f00-5e1d2c3b4a59","input_tokens":1200}`},
		{"keeps lookalike home", "/home/alicex/file", "/home/alicex/file"},
		{"keeps prose", "basic setup complete for bearer token", "basic setup complete for bearer token"},
	}

	r := NewRedactor("/home/alice", "devbox-01")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(r.Redact([]byte(tc.in))); got != tc.want {
				t.Fatalf("Redact(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
