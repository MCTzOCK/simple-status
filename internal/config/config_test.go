package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validYAML = `
title: Example Status
listen: 127.0.0.1:9999
interval: 15s
timeout: 3s
retries: 2
history: 100
services:
  - id: api
    type: http
    target: https://api.example.com/health
    expected_statuses: [200, 204]
  - id: db
    name: Database
    group: Backend
    type: tcp
    target: db.internal:5432
    interval: 60s
  - id: dns
    type: dns
    target: example.com
    record: mx
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppliesFileValuesAndDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, validYAML), Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Title != "Example Status" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if cfg.Interval.Duration != 15*time.Second {
		t.Errorf("Interval = %s", cfg.Interval)
	}
	if cfg.History != 100 || cfg.Retries != 2 {
		t.Errorf("History/Retries = %d/%d", cfg.History, cfg.Retries)
	}

	api := cfg.Services[0]
	if api.Interval.Duration != 15*time.Second { // inherited from root
		t.Errorf("api.Interval = %s, want inherited 15s", api.Interval)
	}
	if api.Method != "GET" {
		t.Errorf("api.Method = %q, want default GET", api.Method)
	}

	db := cfg.Services[1]
	if db.Interval.Duration != 60*time.Second { // explicit service value
		t.Errorf("db.Interval = %s, want 60s", db.Interval)
	}
	if db.Timeout.Duration != 3*time.Second { // inherited
		t.Errorf("db.Timeout = %s, want inherited 3s", db.Timeout)
	}
	if db.Name != "Database" {
		t.Errorf("db.Name = %q", db.Name)
	}

	dns := cfg.Services[2]
	if dns.RecordType != "MX" {
		t.Errorf("dns.RecordType = %q, want normalized MX", dns.RecordType)
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "services:\n  - id: a\n    type: tcp\n    target: h:1\n"), Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Title != DefaultTitle || cfg.Listen != DefaultListen {
		t.Errorf("Title/Listen = %q/%q", cfg.Title, cfg.Listen)
	}
	if cfg.Interval.Duration != DefaultInterval || cfg.Timeout.Duration != DefaultTimeout {
		t.Errorf("Interval/Timeout = %s/%s", cfg.Interval, cfg.Timeout)
	}
	if cfg.Retries != DefaultRetries || cfg.History != DefaultHistory {
		t.Errorf("Retries/History = %d/%d", cfg.Retries, cfg.History)
	}
	s := cfg.Services[0]
	if s.Name != "a" {
		t.Errorf("Name = %q, want fallback to id", s.Name)
	}
	if s.Retries != DefaultRetries {
		t.Errorf("service Retries = %d, want inherited %d", s.Retries, DefaultRetries)
	}
}

func TestLoadFlagOverrides(t *testing.T) {
	cfg, err := Load(writeConfig(t, validYAML), Overrides{
		Listen:   ":1234",
		Interval: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":1234" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if cfg.Interval.Duration != 5*time.Second {
		t.Errorf("Interval = %s, want flag value 5s", cfg.Interval)
	}
	if cfg.Services[1].Interval.Duration != 60*time.Second {
		t.Errorf("explicit service interval lost: %s", cfg.Services[1].Interval)
	}
	if cfg.Services[0].Interval.Duration != 5*time.Second {
		t.Errorf("inherited service interval = %s, want flag value", cfg.Services[0].Interval)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	_, err := Load(writeConfig(t, "interval: [not, a, duration]\n"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("want parse error, got %v", err)
	}
	_, err = Load(writeConfig(t, "interval: banana\n"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("want duration error, got %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yml"), Overrides{}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestExampleConfigStaysValid guards the shipped example against drift:
// it must always parse, normalize and validate.
func TestExampleConfigStaysValid(t *testing.T) {
	cfg, err := Load("../../config.example.yml", Overrides{})
	if err != nil {
		t.Fatalf("Load example config: %v", err)
	}
	if err := cfg.Validate([]string{"http", "tcp", "dns"}); err != nil {
		t.Fatalf("example config no longer valid: %v", err)
	}
	if len(cfg.Services) == 0 {
		t.Fatal("example config lists no services")
	}
}

func TestValidate(t *testing.T) {
	known := []string{"http", "tcp", "dns"}

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"valid", validYAML, ""},
		{"no services", "title: x\n", "at least one service"},
		{"bad id", "services:\n  - id: \"bad id!\"\n    type: tcp\n    target: h:1\n", "id must match"},
		{"unknown type", "services:\n  - id: a\n    type: smtp\n    target: h:1\n", "unknown type"},
		{"missing target", "services:\n  - id: a\n    type: tcp\n", "target is required"},
		{"duplicate id", "services:\n  - id: a\n    type: tcp\n    target: h:1\n  - id: a\n    type: tcp\n    target: h:2\n", "duplicate id"},
		{"timeout exceeds interval", "services:\n  - id: a\n    type: tcp\n    target: h:1\n    interval: 5s\n    timeout: 10s\n", "must not exceed interval"},
		{"bad body pattern", "services:\n  - id: a\n    type: http\n    target: https://x.example\n    body_pattern: \"([unclosed\"\n", "invalid body_pattern"},
		{"bad status", "services:\n  - id: a\n    type: http\n    target: https://x.example\n    expected_statuses: [42]\n", "outside 100-599"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(writeConfig(t, tc.yaml), Overrides{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			err = cfg.Validate(known)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
