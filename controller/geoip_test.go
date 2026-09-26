package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"service/db"
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

func TestCachedValueLeavesOutTheIP(t *testing.T) {
	const ip = "203.0.113.7"
	city := &db.City{IP: ip, Updated: "u"}
	country := &db.Country{IP: ip, Updated: "u"}
	for name, v := range map[string]any{"city": withoutCityIP(city), "country": withoutCountryIP(country)} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), ip) {
			t.Errorf("%s: cached value contains the IP: %s", name, b)
		}
		if !strings.Contains(string(b), `"Updated":"u"`) {
			t.Errorf("%s: cached value lost the other fields: %s", name, b)
		}
	}
	// The response still carries it.
	if city.IP != ip || country.IP != ip {
		t.Error("stripping the cached copy changed the response value")
	}
}
