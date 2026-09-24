package controller

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

func TestInvalidIPIsNotEchoed(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	defer log.Replace(zap.New(core))()

	// Looks like a documentation address (RFC 5737) but does not parse.
	const input = "203.0.113.700"
	c := &GeoIPController{}
	for name, h := range map[string]http.HandlerFunc{"city": c.City, "country": c.Country} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/v1/"+name+"?ip="+input, nil))
		if rec.Code != 400 {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "203.0.113") {
			t.Errorf("%s: 400 body echoes the input: %q", name, rec.Body.String())
		}
	}
	for _, e := range logs.All() {
		if strings.Contains(e.Message, "203.0.113") {
			t.Errorf("log entry contains the input: %s", e.Message)
		}
	}
}
