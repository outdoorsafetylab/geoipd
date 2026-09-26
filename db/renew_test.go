package db

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"service/log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// testdata/old.mmdb and new.mmdb map 203.0.113.0/24 to JP and TW.

const edition = "GeoLite2-Country"

var now = time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

// memStorage is a CloudStorage holding one object in memory.
type memStorage struct {
	data      []byte
	etag      string
	stored    time.Time
	downloads int
	uploads   int
	uploaded  []byte
}

func (m *memStorage) UploadWithMetadata(key string, r io.Reader, meta map[string]string) error {
	b, err := io.ReadAll(r)
	m.uploads++
	m.uploaded = b
	return err
}
func (m *memStorage) Download(string) (io.ReadCloser, error) {
	m.downloads++
	return io.NopCloser(bytes.NewReader(m.data)), nil
}
func (m *memStorage) GetMetadata(string) (map[string]string, error) {
	return map[string]string{"etag": m.etag}, nil
}
func (m *memStorage) Exists(string) (bool, error)               { return m.data != nil, nil }
func (m *memStorage) GetLastModified(string) (time.Time, error) { return m.stored, nil }

func readFixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// maxMindServer serves mmdb as the tar.gz MaxMind ships, answers 304 to a
// matching If-None-Match, and returns status instead when it is non-zero.
type maxMindServer struct {
	*httptest.Server
	hits        int
	ifNoneMatch []string
}

func maxMind(t *testing.T, mmdb []byte, status int) *maxMindServer {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: edition + "_20260925/" + edition + ".mmdb", Mode: 0644, Size: int64(len(mmdb)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(mmdb)
	_ = tw.Close()
	_ = gz.Close()
	m := &maxMindServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits++
		m.ifNoneMatch = append(m.ifNoneMatch, r.Header.Get("If-None-Match"))
		switch {
		case status != 0:
			w.WriteHeader(status)
		case r.Header.Get("If-None-Match") == "maxmind-v2":
			w.WriteHeader(304)
		default:
			w.Header().Set("Etag", "maxmind-v2")
			_, _ = w.Write(buf.Bytes())
		}
	}))
	t.Cleanup(m.Close)
	return m
}

// observeLogs routes the package logger to an observer for this test.
func observeLogs(t *testing.T) *observer.ObservedLogs {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(log.Replace(zap.New(core)))
	return logs
}

func newTestDB(t *testing.T, cloud *memStorage, url string) *geoIP2DB {
	d := &geoIP2DB{
		licenseKey:  "test-license-key",
		edition:     edition,
		maxAge:      7 * 24 * time.Hour,
		downloadURL: url + "/download?edition_id=%s&license_key=%s",
		now:         func() time.Time { return now },
	}
	if cloud != nil {
		d.cloudStorage = cloud
	}
	t.Cleanup(func() {
		if d.reader != nil {
			d.reader.Close()
		}
		if d.path != "" {
			os.Remove(d.path)
		}
	})
	return d
}

func country(t *testing.T, d *geoIP2DB) string {
	if d.reader == nil {
		t.Fatal("no database open")
	}
	c, err := d.reader.Country(net.ParseIP("203.0.113.7"))
	if err != nil {
		t.Fatal(err)
	}
	return c.Country.IsoCode
}

func TestFreshCloudCopyIsUsedWithoutAskingMaxMind(t *testing.T) {
	observeLogs(t)
	mm := maxMind(t, readFixture(t, "new.mmdb"), 0)
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "maxmind-v1", stored: now.Add(-24 * time.Hour)}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if mm.hits != 0 {
		t.Errorf("MaxMind asked %d times for a fresh cloud copy", mm.hits)
	}
	if got := country(t, d); got != "JP" {
		t.Errorf("serving %s, want the cloud copy (JP)", got)
	}
}

func TestRenewTickerAsksMaxMindWithTheCloudETag(t *testing.T) {
	observeLogs(t)
	mm := maxMind(t, readFixture(t, "new.mmdb"), 0)
	cloud := &memStorage{data: readFixture(t, "new.mmdb"), etag: "maxmind-v2", stored: now.Add(-24 * time.Hour)}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	// The ticker: the cloud copy is unchanged, so MaxMind is asked, with the
	// ETag the cloud copy came with, and its 304 keeps the open database.
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if mm.hits != 1 || mm.ifNoneMatch[0] != "maxmind-v2" {
		t.Errorf("MaxMind asked %d times with If-None-Match %q, want once with the cloud ETag", mm.hits, mm.ifNoneMatch)
	}
	if cloud.downloads != 1 {
		t.Errorf("cloud copy fetched %d times, want 1", cloud.downloads)
	}
	if got := country(t, d); got != "TW" {
		t.Errorf("serving %s, want TW", got)
	}
}

func TestStaleCloudCopyIsReplacedWithoutFetchingIt(t *testing.T) {
	observeLogs(t)
	mm := maxMind(t, readFixture(t, "new.mmdb"), 0)
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "maxmind-v1", stored: now.Add(-30 * 24 * time.Hour)}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if mm.hits != 1 {
		t.Errorf("MaxMind asked %d times, want 1", mm.hits)
	}
	// Only one database file on disk at a time: the stale copy is not fetched.
	if cloud.downloads != 0 {
		t.Errorf("stale cloud copy fetched %d times", cloud.downloads)
	}
	if got := country(t, d); got != "TW" {
		t.Errorf("serving %s, want the MaxMind copy (TW)", got)
	}
	if cloud.uploads != 1 || !bytes.Equal(cloud.uploaded, readFixture(t, "new.mmdb")) {
		t.Errorf("new copy not stored back: %d uploads", cloud.uploads)
	}
}

func TestStaleCloudCopyIsTheFallback(t *testing.T) {
	for name, tc := range map[string]struct {
		mmdb   []byte
		status int
	}{
		"http error":   {nil, 500},
		"not modified": {nil, 304},
		"corrupt file": {[]byte("not a database"), 0},
	} {
		t.Run(name, func(t *testing.T) {
			observeLogs(t)
			mm := maxMind(t, tc.mmdb, tc.status)
			cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "maxmind-v1", stored: now.Add(-30 * 24 * time.Hour)}
			d := newTestDB(t, cloud, mm.URL)
			if err := d.renew(); err != nil {
				t.Fatalf("renew failed instead of falling back: %v", err)
			}
			if got := country(t, d); got != "JP" {
				t.Errorf("serving %s, want the cloud copy (JP)", got)
			}
			if cloud.uploads != 0 {
				t.Errorf("stored %d copies back without a usable download", cloud.uploads)
			}
		})
	}
}

func TestNoDatabaseAnywhereIsAnError(t *testing.T) {
	observeLogs(t)
	mm := maxMind(t, nil, 304)
	d := newTestDB(t, &memStorage{}, mm.URL)
	if err := d.renew(); err == nil {
		t.Fatal("renew succeeded with no database open")
	}
}

func TestDownloadErrorDoesNotLeakLicenseKey(t *testing.T) {
	logs := observeLogs(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused

	// With a cloud copy the failure is logged and the copy used.
	stale := newTestDB(t, &memStorage{data: readFixture(t, "old.mmdb"), etag: "e", stored: now.Add(-30 * 24 * time.Hour)}, url)
	if err := stale.renew(); err != nil {
		t.Fatal(err)
	}
	// With nothing to fall back on the failure is returned.
	err := newTestDB(t, nil, url).renew()
	if err == nil {
		t.Fatal("renew with no DB at all succeeded")
	}
	if strings.Contains(err.Error(), "test-license-key") {
		t.Errorf("error leaks the license key: %v", err)
	}
	if logs.Len() == 0 {
		t.Fatal("no log lines seen; the failure path was not exercised")
	}
	for _, e := range logs.All() {
		if strings.Contains(e.Message, "test-license-key") {
			t.Errorf("log leaks the license key: %s", e.Message)
		}
	}
}
