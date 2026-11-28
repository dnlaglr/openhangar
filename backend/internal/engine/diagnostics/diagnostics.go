package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/shared/logging"
	"github.com/dnlaglr/openhangar/backend/internal/shared/telemetry"
)

type Options struct {
	LogDir       string
	Redactor     *logging.Redactor
	AppVersion   string
	GuestVersion string
	WSLVersion   string
	Runtime      string
}

type System struct {
	GeneratedAt       time.Time `json:"generated_at"`
	AppVersion        string    `json:"app_version"`
	Engine            Build     `json:"engine"`
	GuestVersion      string    `json:"guest_version"`
	OS                string    `json:"os"`
	OSRelease         string    `json:"os_release"`
	Kernel            string    `json:"kernel"`
	WSLVersion        string    `json:"wsl_version"`
	Arch              string    `json:"arch"`
	CPUs              int       `json:"cpus"`
	Runtime           string    `json:"runtime"`
	KVM               string    `json:"kvm"`
	TelemetryDisabled bool      `json:"telemetry_disabled"`
}

type Build struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Time     string `json:"time"`
	Modified bool   `json:"modified"`
	Go       string `json:"go"`
}

func Write(w io.Writer, opts Options) error {
	sys, err := json.MarshalIndent(collect(opts), "", "  ")
	if err != nil {
		return err
	}

	zw := zip.NewWriter(w)
	if err := add(zw, "system.json", opts.Redactor.Redact(sys), time.Now()); err != nil {
		return err
	}

	if err := addLogs(zw, opts); err != nil {
		return err
	}

	return zw.Close()
}

func addLogs(zw *zip.Writer, opts Options) error {
	return filepath.WalkDir(opts.LogDir, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == logging.RawDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(opts.LogDir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}

		return add(zw, "logs/"+filepath.ToSlash(rel), opts.Redactor.Redact(data), info.ModTime())
	})
}

func add(zw *zip.Writer, name string, data []byte, modified time.Time) error {
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
	if err != nil {
		return err
	}
	_, err = f.Write(data)

	return err
}

func collect(opts Options) System {
	return System{
		GeneratedAt:       time.Now().UTC(),
		AppVersion:        opts.AppVersion,
		Engine:            engineBuild(),
		GuestVersion:      opts.GuestVersion,
		OS:                runtime.GOOS,
		OSRelease:         osRelease(),
		Kernel:            kernel(),
		WSLVersion:        opts.WSLVersion,
		Arch:              runtime.GOARCH,
		CPUs:              runtime.NumCPU(),
		Runtime:           opts.Runtime,
		KVM:               kvm(),
		TelemetryDisabled: telemetry.Disabled(),
	}
}

func engineBuild() Build {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Build{}
	}

	b := Build{Version: info.Main.Version, Go: info.GoVersion}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
		case "vcs.time":
			b.Time = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}

	return b
}

func osRelease() string {
	data, _ := os.ReadFile("/etc/os-release")
	for line := range strings.Lines(string(data)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}

	return ""
}

func kernel() string {
	data, _ := os.ReadFile("/proc/sys/kernel/osrelease")

	return strings.TrimSpace(string(data))
}

func kvm() string {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	switch {
	case err == nil:
		f.Close()
		return "available"
	case errors.Is(err, fs.ErrNotExist):
		return "missing"
	case errors.Is(err, fs.ErrPermission):
		return "no access"
	default:
		return "error"
	}
}
