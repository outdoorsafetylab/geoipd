package db

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
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
	readErr   error // makes the body fail partway
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
	m.data, m.etag, m.stored = b, meta["etag"], now
	return err
}
func (m *memStorage) Download(string) (io.ReadCloser, error) {
	m.downloads++
	if m.readErr != nil {
		return io.NopCloser(io.MultiReader(bytes.NewReader(m.data[:10]), errReader{m.readErr})), nil
	}
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
	return maxMindCut(t, mmdb, status, 0)
}

// maxMindCut is maxMind sending only the first cut bytes of the archive when
// cut is non-zero.
func maxMindCut(t *testing.T, mmdb []byte, status, cut int) *maxMindServer {
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
			body := buf.Bytes()
			if cut > 0 {
				if cut >= len(body) {
					t.Errorf("cut %d is not inside the %d-byte archive", cut, len(body))
				} else {
					body = body[:cut]
				}
			}
			_, _ = w.Write(body)
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
		client:      &http.Client{Timeout: 5 * time.Second},
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
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
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
			if left, _ := os.ReadDir(tmp); len(left) != 1 {
				t.Errorf("%d files in the temp dir, want only the one in service", len(left))
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

// A cloud copy whose body cannot be used must not become the version in
// service: its ETag would make MaxMind answer 304 and later renews skip the
// cloud, leaving the old database in place for good.
func TestUnusableCloudUpdateDoesNotTakeOverTheETag(t *testing.T) {
	observeLogs(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	mm := maxMind(t, readFixture(t, "new.mmdb"), 0) // 304 for "maxmind-v2"
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "maxmind-v1", stored: now.Add(-24 * time.Hour)}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	// Another instance stored v2, but this body is broken.
	cloud.data, cloud.etag = []byte("not a database"), "maxmind-v2"
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	// MaxMind was asked with the ETag of the copy in service (v1), not the
	// unusable cloud one, so it sent v2 instead of a 304.
	if len(mm.ifNoneMatch) != 1 || mm.ifNoneMatch[0] != "maxmind-v1" {
		t.Errorf("If-None-Match sent: %q, want [maxmind-v1]", mm.ifNoneMatch)
	}
	if got := country(t, d); got != "TW" {
		t.Errorf("serving %s, want TW from MaxMind", got)
	}
	// Only the database in service is left on disk.
	if left, _ := os.ReadDir(tmp); len(left) != 1 {
		t.Errorf("%d files in the temp dir, want only the one in service", len(left))
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// A fetch that fails partway leaves no temp file behind: /tmp is memory on
// Cloud Run.
func TestFailedFetchLeavesNoTempFile(t *testing.T) {
	observeLogs(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	mm := maxMind(t, nil, 500)
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "e", stored: now.Add(-24 * time.Hour), readErr: io.ErrUnexpectedEOF}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err == nil {
		t.Fatal("renew succeeded with no usable database")
	}
	left, _ := os.ReadDir(tmp)
	if len(left) != 0 {
		t.Errorf("%d temp files left behind", len(left))
	}
}

func TestTruncatedDownloadLeavesNoTempFile(t *testing.T) {
	observeLogs(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	// An incompressible payload, so cutting the archive in half lands inside
	// the file body.
	payload := make([]byte, 64<<10)
	_, _ = rand.Read(payload)
	mm := maxMindCut(t, payload, 0, 32<<10)
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "e", stored: now.Add(-30 * 24 * time.Hour)}
	d := newTestDB(t, cloud, mm.URL)
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if got := country(t, d); got != "JP" {
		t.Errorf("serving %s, want the cloud copy (JP)", got)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 1 {
		t.Errorf("%d files in the temp dir, want only the one in service", len(left))
	}
}

// A MaxMind that accepts the connection and then goes silent must end in the
// fallback, not hold startup.
func TestStalledDownloadFallsBack(t *testing.T) {
	observeLogs(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "e", stored: now.Add(-30 * 24 * time.Hour)}
	d := newTestDB(t, cloud, srv.URL)
	d.client = &http.Client{Timeout: 200 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- d.renew() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("renew still waiting on a silent MaxMind")
	}
	if got := country(t, d); got != "JP" {
		t.Errorf("serving %s, want the cloud copy (JP)", got)
	}
}

// Once a database is in service, a failed refresh keeps it: the ticker must
// never take down or replace a working database with nothing.
func TestFailedRefreshKeepsTheOpenDatabase(t *testing.T) {
	for name, tc := range map[string]struct {
		mmdb   []byte
		status int
	}{
		"http error":   {nil, 500},
		"corrupt file": {[]byte("not a database"), 0},
	} {
		t.Run(name, func(t *testing.T) {
			observeLogs(t)
			mm := maxMind(t, tc.mmdb, tc.status)
			cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "maxmind-v1", stored: now.Add(-24 * time.Hour)}
			d := newTestDB(t, cloud, mm.URL)
			if err := d.renew(); err != nil {
				t.Fatal(err)
			}
			cloud.data = nil // the cloud copy is gone by the time the ticker fires
			if err := d.renew(); err != nil {
				t.Fatalf("refresh failed the running service: %v", err)
			}
			if mm.hits != 1 {
				t.Errorf("MaxMind asked %d times, want 1", mm.hits)
			}
			if got := country(t, d); got != "JP" {
				t.Errorf("serving %s, want the open database (JP)", got)
			}
			if cloud.uploads != 0 {
				t.Errorf("stored %d copies back from a failed refresh", cloud.uploads)
			}
		})
	}
}
