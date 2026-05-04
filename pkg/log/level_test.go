package log

import (
	"log/slog"
	"testing"
)

func TestLevelKnob_Defaults(t *testing.T) {
	k := NewLevelKnob("")
	if k.Level() != slog.LevelInfo {
		t.Errorf("empty level should default to info, got %v", k.Level())
	}
	if k.String() != "info" {
		t.Errorf("String() = %q, want info", k.String())
	}
}

func TestLevelKnob_SetParses(t *testing.T) {
	k := NewLevelKnob("info")
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERR", slog.LevelError},
		{"  info  ", slog.LevelInfo}, // trim
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if err := k.Set(tc.in); err != nil {
				t.Fatalf("Set(%q) failed: %v", tc.in, err)
			}
			if k.Level() != tc.want {
				t.Errorf("Set(%q) -> Level = %v, want %v", tc.in, k.Level(), tc.want)
			}
		})
	}
}

func TestLevelKnob_SetRejectsUnknown(t *testing.T) {
	k := NewLevelKnob("info")
	if err := k.Set("verbose"); err == nil {
		t.Error("expected error on unknown level, got nil — bad input must surface so the API can return 400")
	}
	// Level should remain unchanged after a rejected Set.
	if k.Level() != slog.LevelInfo {
		t.Errorf("level changed after rejected Set: %v", k.Level())
	}
}
