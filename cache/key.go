package cache

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
)

// keySecret is drawn once per process. Each container runs its own redis, so
// no other instance needs to reproduce a key, and a restart only costs cache
// misses. Nothing stores the secret, so a key cannot be traced back to an IP.
var keySecret = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}()

// IPKey returns the cache key for a lookup of ip. It is an HMAC of the IP rather
// than the IP itself: a plain SHA-256 of an IPv4 address is reversible by trying
// all 2^32 of them.
func IPKey(kind string, ip net.IP) string {
	m := hmac.New(sha256.New, keySecret)
	m.Write([]byte(ip.String()))
	return kind + ":" + hex.EncodeToString(m.Sum(nil))
}
