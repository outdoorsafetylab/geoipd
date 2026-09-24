package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The cache-hit, lookup and lookup-failure paths need redis and a GeoIP
// database, so only the 400 path is covered here.
func TestInvalidIPIsNotEchoed(t *testing.T) {
	// Looks like a documentation address (RFC 5737) but does not parse.
	const input = "203.0.113.700"
	c := &GeoIPController{}
	for name, h := range map[string]http.HandlerFunc{"city": c.City, "country": c.Country} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/v1/"+name+"?ip="+input, nil))
		if rec.Code != 400 {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
		if got := rec.Body.String(); got != "Invalid IP address\n" {
			t.Errorf("%s: 400 body = %q, want the fixed message", name, got)
		}
	}
}
