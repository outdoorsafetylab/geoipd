package cache

import (
	"net"
	"strings"
	"testing"
)

func TestIPKeyHidesTheIP(t *testing.T) {
	// Documentation addresses (RFC 5737, RFC 3849).
	for _, s := range []string{"203.0.113.7", "2001:db8::7"} {
		ip := net.ParseIP(s)
		k := IPKey("country", ip)
		if strings.Contains(k, s) {
			t.Errorf("key %q contains the IP", k)
		}
		if !strings.HasPrefix(k, "country:") || len(k) != len("country:")+64 {
			t.Errorf("key %q is not country:<hmac-sha256 hex>", k)
		}
		if k != IPKey("country", ip) {
			t.Errorf("key for %s is not stable within a process", s)
		}
	}
}

func TestIPKeySeparatesIPsAndKinds(t *testing.T) {
	a, b := net.ParseIP("203.0.113.7"), net.ParseIP("203.0.113.8")
	if IPKey("country", a) == IPKey("country", b) {
		t.Error("two IPs share a key")
	}
	if IPKey("country", a) == IPKey("city", a) {
		t.Error("city and country share a key")
	}
	// An IPv4 address and its IPv4-mapped IPv6 form are the same lookup.
	if IPKey("country", a) != IPKey("country", net.ParseIP("::ffff:203.0.113.7")) {
		t.Error("IPv4-mapped form gets a different key")
	}
}

func TestIPKeyDependsOnTheSecret(t *testing.T) {
	ip := net.ParseIP("203.0.113.7")
	before := IPKey("country", ip)
	prev := keySecret
	keySecret = []byte("another secret")
	defer func() { keySecret = prev }()
	// An unkeyed hash of the IP would not move, and could be reversed by
	// hashing every IPv4 address.
	if IPKey("country", ip) == before {
		t.Error("key does not depend on the secret")
	}
}
