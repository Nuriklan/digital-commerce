package logger_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nuriklan/digital-commerce/pkg/logger"
)

func TestMaskedString_LogValue(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Config{
		Level:  logger.LevelInfo,
		Format: logger.FormatJSON,
		Output: &buf,
	})

	secretPassword := logger.MaskedString("super_secret_password_123")
	l.Info("user login attempt", "password", secretPassword)

	raw := buf.String()
	if strings.Contains(raw, "super_secret_password_123") {
		t.Fatalf("plain secret password was leaked in logs: %s", raw)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse json log: %v", err)
	}

	if entry["password"] != "******" {
		t.Errorf("expected masked value '******', got: %v", entry["password"])
	}
}

func TestMaskedCard_LogValue(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Config{
		Level:  logger.LevelInfo,
		Format: logger.FormatJSON,
		Output: &buf,
	})

	card := logger.MaskedCard("4111222233334444")
	l.Info("processing card payment", "card_number", card)

	raw := buf.String()
	if strings.Contains(raw, "22223333") {
		t.Fatalf("unmasked card digits leaked in log: %s", raw)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse json log: %v", err)
	}

	expected := "4111********4444"
	if entry["card_number"] != expected {
		t.Errorf("expected card %q, got %q", expected, entry["card_number"])
	}
}

func TestLogLevel_Filter(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Config{
		Level:  logger.LevelWarn,
		Format: logger.FormatJSON,
		Output: &buf,
	})

	l.Debug("debug message")
	l.Info("info message")
	if buf.Len() > 0 {
		t.Fatalf("expected debug and info to be filtered out at WARN level, got: %s", buf.String())
	}

	l.Warn("warning message")
	if !strings.Contains(buf.String(), "warning message") {
		t.Errorf("expected warning message in log output")
	}
}
