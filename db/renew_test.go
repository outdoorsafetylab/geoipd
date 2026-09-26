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

// testdata/old.mmdb and new.mmdb map 203.0.113.0/24 to JP and TW, built at
// these epochs.
var (
	oldBuilt = time.Unix(1700000000, 0)
	newBuilt = time.Unix(1800000000, 0)
)

const edition = "GeoLite2-Country"

// memStorage is a CloudStorage holding one object in memory.
type memStorage struct {
	data     []byte
	etag     string
	uploads  int
	uploaded []byte
}

func (m *memStorage) UploadWithMetadata(key string, r io.Reader, meta map[string]string) error {
	b, err := io.ReadAll(r)
	m.uploads++
	m.uploaded = b
	return err
}
func (m *memStorage) Download(string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.data)), nil
}
func (m *memStorage) GetMetadata(string) (map[string]string, error) {
	return map[string]string{"etag": m.etag}, nil
}
func (m *memStorage) Exists(string) (bool, error) { return m.data != nil, nil }
func (m *memStorage) GetLastModified(string) (time.Time, error) {
	return time.Now(), nil
}

func readFixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// maxMind serves mmdb as the tar.gz MaxMind ships, or status when non-zero.
func maxMind(t *testing.T, mmdb []byte, status int, hits *int) *httptest.Server {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: edition + "_20270115/" + edition + ".mmdb", Mode: 0644, Size: int64(len(mmdb)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(mmdb)
	_ = tw.Close()
	_ = gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Etag", "maxmind-new")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv
}

// observeLogs routes the package logger to an observer for this test.
func observeLogs(t *testing.T) *observer.ObservedLogs {
	core, logs := observer.New(zapcore.DebugLevel)
	t.Cleanup(log.Replace(zap.New(core)))
	return logs
}

func newTestDB(t *testing.T, cloud *memStorage, url string, now time.Time) *geoIP2DB {
	d := &geoIP2DB{
		licenseKey:   "test-license-key",
		edition:      edition,
		cloudStorage: cloud,
		maxAge:       7 * 24 * time.Hour,
		downloadURL:  url + "/download?edition_id=%s&license_key=%s",
		now:          func() time.Time { return now },
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
	c, err := d.reader.Country(net.ParseIP("203.0.113.7"))
	if err != nil {
		t.Fatal(err)
	}
	return c.Country.IsoCode
}

func TestFreshCloudCopyIsUsedWithoutAskingMaxMind(t *testing.T) {
	observeLogs(t)
	hits := 0
	srv := maxMind(t, readFixture(t, "new.mmdb"), 0, &hits)
	cloud := &memStorage{data: readFixture(t, "new.mmdb"), etag: "e"}
	d := newTestDB(t, cloud, srv.URL, newBuilt.Add(24*time.Hour))
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("MaxMind asked %d times for a fresh cloud copy", hits)
	}
	if got := country(t, d); got != "TW" {
		t.Errorf("serving %s, want the cloud copy (TW)", got)
	}
}

func TestStaleCloudCopyIsReplacedFromMaxMind(t *testing.T) {
	observeLogs(t)
	hits := 0
	srv := maxMind(t, readFixture(t, "new.mmdb"), 0, &hits)
	cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "e"}
	d := newTestDB(t, cloud, srv.URL, oldBuilt.Add(30*24*time.Hour))
	if err := d.renew(); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("MaxMind asked %d times, want 1", hits)
	}
	if got := country(t, d); got != "TW" {
		t.Errorf("serving %s, want the MaxMind copy (TW)", got)
	}
	if cloud.uploads != 1 || !bytes.Equal(cloud.uploaded, readFixture(t, "new.mmdb")) {
		t.Errorf("new copy not stored back: %d uploads", cloud.uploads)
	}
}

func TestStaleCloudCopyIsKeptWhenMaxMindFails(t *testing.T) {
	for name, status := range map[string]int{"http error": 500, "not modified": 304} {
		t.Run(name, func(t *testing.T) {
			observeLogs(t)
			hits := 0
			srv := maxMind(t, nil, status, &hits)
			cloud := &memStorage{data: readFixture(t, "old.mmdb"), etag: "e"}
			d := newTestDB(t, cloud, srv.URL, oldBuilt.Add(30*24*time.Hour))
			if err := d.renew(); err != nil {
				t.Fatalf("renew failed instead of keeping the cloud copy: %v", err)
			}
			if hits != 1 {
				t.Errorf("MaxMind asked %d times, want 1", hits)
			}
			if got := country(t, d); got != "JP" {
				t.Errorf("serving %s, want the cloud copy (JP)", got)
			}
			if cloud.uploads != 0 {
				t.Errorf("stored %d copies back without a new download", cloud.uploads)
			}
		})
	}
}

func TestDownloadErrorDoesNotLeakLicenseKey(t *testing.T) {
	logs := observeLogs(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused

	// With a stale cloud copy the failure is logged and the copy kept.
	stale := newTestDB(t, &memStorage{data: readFixture(t, "old.mmdb"), etag: "e"}, url, oldBuilt.Add(30*24*time.Hour))
	if err := stale.renew(); err != nil {
		t.Fatal(err)
	}
	// With nothing to fall back on the failure is returned.
	empty := newTestDB(t, &memStorage{}, url, time.Now())
	err := empty.renew()
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
