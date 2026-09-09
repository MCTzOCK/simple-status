package prober

import (
	"strings"
	"testing"

	"simple-status/internal/config"
)

func TestRegistry(t *testing.T) {
	want := []string{"dns", "http", "tcp"}
	got := Types()
	if len(got) != len(want) {
		t.Fatalf("Types() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Types() = %v, want %v", got, want)
		}
	}
}

func TestNewUnknownType(t *testing.T) {
	_, err := New(config.Service{ID: "x", Type: "carrier-pigeon", Target: "coop"})
	if err == nil || !strings.Contains(err.Error(), "unknown probe type") {
		t.Fatalf("want unknown type error, got %v", err)
	}
}

func TestNewBuildsRegisteredTypes(t *testing.T) {
	if _, err := New(config.Service{ID: "x", Type: "tcp", Target: "example.com:80"}); err != nil {
		t.Fatalf("New(tcp): %v", err)
	}
}
