package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const RawDir = "raw"

func New(w io.Writer, level slog.Leveler, r *Redactor) *slog.Logger {
	h := slog.NewJSONHandler(r.Writer(w), &slog.HandlerOptions{Level: level})

	return slog.New(contextHandler{h})
}

func Dir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(cache, "hangar", "logs"), nil
}

func File(path string) io.WriteCloser {
	return &lumberjack.Logger{Filename: path, MaxSize: 10, MaxBackups: 5, MaxAge: 14}
}

func Raw(w io.Writer, level slog.Leveler, r *Redactor) io.Writer {
	return rawWriter{w: r.Writer(w), level: level}
}

type rawWriter struct {
	w     io.Writer
	level slog.Leveler
}

func (rw rawWriter) Write(p []byte) (int, error) {
	if rw.level.Level() > slog.LevelDebug {
		return len(p), nil
	}

	return rw.w.Write(p)
}
