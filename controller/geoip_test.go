package controller

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"service/cache"
	"service/db"
	"service/log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

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

// fakeCache stands in for redis, keeping the bytes the handler would store.
func fakeCache(t *testing.T) map[string][]byte {
	store := map[string][]byte{}
	prevGet, prevSet := cacheGet, cacheSet
	cacheGet = func(key string, v interface{}) error {
		b, ok := store[key]
		if !ok {
			return cache.Miss
		}
		return json.Unmarshal(b, v)
	}
	cacheSet = func(key string, v interface{}) error {
		b, err := json.Marshal(v)
		store[key] = b
		return err
	}
	t.Cleanup(func() { cacheGet, cacheSet = prevGet, prevSet })
	return store
}

func TestCacheHoldsNoIPAndHitsRestoreIt(t *testing.T) {
	const ip = "203.0.113.7"
	core, logs := observer.New(zapcore.DebugLevel)
	defer log.Replace(zap.New(core))()
	store := fakeCache(t)
	queries := 0
	prevCity, prevCountry := queryCity, queryCountry
	queryCity = func(q net.IP) (*db.City, error) {
		queries++
		return &db.City{IP: q.String(), Updated: "u"}, nil
	}
	queryCountry = func(q net.IP) (*db.Country, error) {
		queries++
		return &db.Country{IP: q.String(), Updated: "u"}, nil
	}
	t.Cleanup(func() { queryCity, queryCountry = prevCity, prevCountry })

	c := &GeoIPController{}
	for name, h := range map[string]http.HandlerFunc{"city": c.City, "country": c.Country} {
		for _, pass := range []string{"miss", "hit"} {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest("GET", "/v1/"+name+"?ip="+ip, nil))
			if rec.Code != 200 {
				t.Fatalf("%s %s: status %d", name, pass, rec.Code)
			}
			var got struct{ IP, Updated string }
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("%s %s: %v", name, pass, err)
			}
			if got.IP != ip || got.Updated != "u" {
				t.Errorf("%s %s: response %+v, want IP %s", name, pass, got, ip)
			}
		}
	}
	if queries != 2 {
		t.Errorf("GeoIP database queried %d times, want once per kind", queries)
	}
	if len(store) != 2 {
		t.Fatalf("cache holds %d entries, want one per kind", len(store))
	}
	if logs.Len() == 0 {
		t.Error("no log lines seen; the cache and query messages were not exercised")
	}
	for _, e := range logs.All() {
		if strings.Contains(e.Message, ip) {
			t.Errorf("log entry contains the IP: %s", e.Message)
		}
	}
	for k, v := range store {
		if strings.Contains(k, ip) || strings.Contains(string(v), ip) {
			t.Errorf("cache entry holds the IP: %s = %s", k, v)
		}
	}
}

func TestLookupFailureIsNotEchoed(t *testing.T) {
	const ip = "2001:db8::7"
	fakeCache(t)
	core, logs := observer.New(zapcore.DebugLevel)
	defer log.Replace(zap.New(core))()
	// The GeoIP reader's own error quotes the address.
	lookupErr := fmt.Errorf("error looking up '%s': IPv6 address in an IPv4-only database", ip)
	prevCity, prevCountry := queryCity, queryCountry
	queryCity = func(net.IP) (*db.City, error) { return nil, lookupErr }
	queryCountry = func(net.IP) (*db.Country, error) { return nil, lookupErr }
	t.Cleanup(func() { queryCity, queryCountry = prevCity, prevCountry })

	c := &GeoIPController{}
	for name, h := range map[string]http.HandlerFunc{"city": c.City, "country": c.Country} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/v1/"+name+"?ip="+ip, nil))
		if rec.Code != 500 {
			t.Errorf("%s: status %d, want 500", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "2001:db8") {
			t.Errorf("%s: 500 body echoes the address: %q", name, rec.Body.String())
		}
	}
	for _, e := range logs.All() {
		if strings.Contains(e.Message, "2001:db8") {
			t.Errorf("log entry contains the address: %s", e.Message)
		}
	}
}
