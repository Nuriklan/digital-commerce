package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Config struct {
	Level  Level
	Format Format
	Output io.Writer
}

func New(cfg Config) *slog.Logger {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	var lvl slog.Level
	switch strings.ToUpper(string(cfg.Level)) {
	case string(LevelDebug):
		lvl = slog.LevelDebug
	case string(LevelWarn):
		lvl = slog.LevelWarn
	case string(LevelError):
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}

	var handler slog.Handler
	if cfg.Format == FormatJSON {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}

	return slog.New(handler)
}

func SetDefault(cfg Config) *slog.Logger {
	l := New(cfg)
	slog.SetDefault(l)
	return l
}

// --- PCI-DSS / Security ---

type MaskedString string

func (m MaskedString) LogValue() slog.Value {
	if m == "" {
		return slog.StringValue("")
	}
	return slog.StringValue("******")
}

func (m MaskedString) String() string {
	return string(m)
}

type MaskedCard string

func (c MaskedCard) LogValue() slog.Value {
	pan := strings.ReplaceAll(string(c), " ", "")
	if len(pan) < 8 {
		return slog.StringValue("****")
	}
	prefix := pan[:4]
	suffix := pan[len(pan)-4:]
	maskedPart := strings.Repeat("*", len(pan)-8)
	return slog.StringValue(prefix + maskedPart + suffix)
}

func (c MaskedCard) String() string {
	return string(c)
}
