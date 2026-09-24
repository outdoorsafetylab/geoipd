package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"service/log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Documentation addresses (RFC 5737).
const (
	queriedIP = "203.0.113.7"
	callerIP  = "198.51.100.9"
)

func TestDumpOmitsIPs(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	defer log.Replace(zap.New(core))()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"IP":"` + queriedIP + `"}`))
	})
	req := httptest.NewRequest("GET", "/v1/country?ip="+queriedIP, nil)
	req.Header.Set("X-Forwarded-For", callerIP)
	req.Header.Set("Forwarded", "for="+callerIP)
	req.Header.Set("X-Real-Ip", callerIP)
	rec := httptest.NewRecorder()
	Dump(inner).ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), queriedIP) {
		t.Fatalf("response body was altered: %q", rec.Body.String())
	}
	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want the Handling line and the dump", len(entries))
	}
	for _, e := range entries {
		for _, ip := range []string{queriedIP, callerIP} {
			if strings.Contains(e.Message, ip) {
				t.Errorf("log entry contains %s: %s", ip, e.Message)
			}
		}
	}
	if !strings.Contains(entries[0].Message, "/v1/country") {
		t.Errorf("Handling line lost the path: %s", entries[0].Message)
	}
	if !strings.Contains(entries[1].Message, `"Code":200`) {
		t.Errorf("dump lost the status code: %s", entries[1].Message)
	}
}

func TestDumpKeepsCallerHeadersIntact(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	defer log.Replace(zap.New(core))()

	var seen string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Forwarded-For")
	})
	req := httptest.NewRequest("GET", "/v1/country", nil)
	req.Header.Set("X-Forwarded-For", callerIP)
	Dump(inner).ServeHTTP(httptest.NewRecorder(), req)

	// The handler falls back to this header when no ?ip= is given.
	if seen != callerIP {
		t.Errorf("handler saw X-Forwarded-For %q, want %q", seen, callerIP)
	}
	if got := req.Header.Get("X-Forwarded-For"); got != callerIP {
		t.Errorf("redaction mutated the request header: %q", got)
	}
}
