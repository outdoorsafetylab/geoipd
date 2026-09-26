package log

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want zapcore.Level
	}{
		{"", zapcore.InfoLevel},
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
	} {
		got, err := parseLevel(tc.in)
		if err != nil {
			t.Fatalf("parseLevel(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if _, err := parseLevel("verbose"); err == nil {
		t.Error("parseLevel(\"verbose\") succeeded, want error")
	}
}
