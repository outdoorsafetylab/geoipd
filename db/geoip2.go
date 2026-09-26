package db

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"service/config"
	"service/log"
	"service/storage"

	"github.com/oschwald/geoip2-golang"
)

var db *geoIP2DB
var ticker *time.Ticker
var done chan bool

func Init() error {
	cfg := config.Get()
	key := cfg.GetString("geoip2.license_key")
	if key == "" {
		log.Errorf("Please specify 'geoip2.license_key' in YAML config or set GEOIP2_LICENSE_KEY environment variable in order to download DB.")
		return errors.New("missing license key")
	}
	edition := cfg.GetString("geoip2.edition")
	db = newGeoIP2DB(key, edition)
	err := db.renew()
	if err != nil {
		return err
	}
	renew := cfg.GetString("geoip2.renew")
	if renew != "" {
		du, err := time.ParseDuration(renew)
		if err != nil {
			log.Errorf("Invalid renew duration: %s", renew)
			return err
		}
		log.Infof("Scheduling DB renew every %s", renew)
		ticker = time.NewTicker(du)
		done = make(chan bool)
		go func() {
			for {
				select {
				case <-done:
					return
				case t := <-ticker.C:
					log.Infof("Renewing DB at %s", t.String())
					_ = db.renew()
				}
			}
		}()
	}
	return nil
}

func Deinit() {
	if ticker != nil {
		ticker.Stop()
		done <- true
		ticker = nil
	}
	if db != nil {
		if db.reader != nil {
			db.reader.Close()
			db.reader = nil
		}
		if db.path != "" {
			os.Remove(db.path)
		}
		db = nil
	}
}

type City struct {
	IP      string `json:"IP"`
	Updated string `json:"Updated,omitempty"`
	*geoip2.City
}

func QueryCity(ip net.IP) (*City, error) {
	db.Lock()
	defer db.Unlock()
	res, err := db.reader.City(ip)
	if err != nil {
		return nil, err
	}
	return &City{
		City:    res,
		IP:      ip.String(),
		Updated: db.modTime.Format(time.RFC1123),
	}, nil
}

type Country struct {
	IP      string `json:"IP"`
	Updated string `json:"Updated,omitempty"`
	*geoip2.Country
}

func QueryCountry(ip net.IP) (*Country, error) {
	db.Lock()
	defer db.Unlock()
	res, err := db.reader.Country(ip)
	if err != nil {
		return nil, err
	}
	return &Country{
		Country: res,
		IP:      ip.String(),
		Updated: db.modTime.Format(time.RFC1123),
	}, nil
}

type geoIP2DB struct {
	sync.Mutex
	licenseKey   string
	edition      string
	etag         string
	modTime      time.Time
	path         string
	reader       *geoip2.Reader
	cloudStorage storage.CloudStorage
	// maxAge is how long ago the cloud copy may have been stored before
	// MaxMind is asked for a newer one.
	maxAge time.Duration
	// downloadURL is the MaxMind download URL format: edition, license key.
	downloadURL string
	now         func() time.Time
}

const (
	defaultMaxAge      = 7 * 24 * time.Hour
	maxMindDownloadURL = "https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&suffix=tar.gz"
)

func newGeoIP2DB(licenseKey, edition string) *geoIP2DB {
	cfg := config.Get()
	var cloudStorage storage.CloudStorage

	// Initialize cloud storage if configured
	if cfg.IsSet("geoip2.cloud_storage.provider") {
		storageConfig := &storage.Config{
			Provider:  cfg.GetString("geoip2.cloud_storage.provider"),
			Bucket:    cfg.GetString("geoip2.cloud_storage.bucket"),
			Region:    cfg.GetString("geoip2.cloud_storage.region"),
			KeyPrefix: cfg.GetString("geoip2.cloud_storage.key_prefix"),
		}

		var err error
		cloudStorage, err = storage.NewCloudStorage(storageConfig)
		if err != nil {
			log.Errorf("Failed to initialize cloud storage: %s", err.Error())
			log.Warnf("Falling back to local storage")
			cloudStorage = nil
		} else {
			log.Infof("Initialized cloud storage: %s://%s", storageConfig.Provider, storageConfig.Bucket)
		}
	}

	maxAge := defaultMaxAge
	if v := cfg.GetString("geoip2.max_age"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Errorf("Invalid geoip2.max_age %q, using %s", v, defaultMaxAge)
		} else {
			maxAge = d
		}
	}

	return &geoIP2DB{
		licenseKey:   licenseKey,
		edition:      edition,
		cloudStorage: cloudStorage,
		maxAge:       maxAge,
		downloadURL:  maxMindDownloadURL,
		now:          time.Now,
	}
}

func (db *geoIP2DB) renew() error {
	// A cloud copy that has not been refreshed from MaxMind within maxAge is
	// skipped: on a platform that scales to zero an instance rarely lives until
	// the renew ticker fires, so this is where a stale copy gets replaced.
	// Deciding from the object's timestamp, before its body is fetched, keeps a
	// single database file on disk at a time.
	if db.cloudStorage != nil && !db.cloudCopyStale() {
		if db.openFromCloud() {
			return nil
		}
		// Unchanged since the last load (renew ticker), missing or unusable:
		// ask MaxMind.
	}

	f, err := db.download()
	switch {
	case err != nil:
		log.Warnf("MaxMind download failed: %s", err.Error())
	case f.path == "":
		// 304: MaxMind has nothing newer than the ETag we sent.
	default:
		if err = db.use(f); err == nil {
			// Store only a copy that opened, so a bad download never
			// replaces the cloud copy other instances fall back on.
			if db.cloudStorage != nil {
				if err := db.storeInCloudStorage(f.path); err != nil {
					log.Errorf("Failed to store in cloud storage: %s", err.Error())
				}
			}
			return nil
		}
		os.Remove(f.path)
	}

	// MaxMind gave nothing usable. Keep what is open, else fall back to the
	// cloud copy however old it is.
	if db.hasReader() {
		return nil
	}
	if db.cloudStorage != nil && db.openFromCloud() {
		return nil
	}
	if err == nil {
		err = errors.New("no GeoIP database available")
	}
	return err
}

// cloudCopyStale reports whether the cloud copy was last stored longer ago than
// maxAge. A copy is stored only after a MaxMind download, so this is the time
// since the last refresh. Any error reads as not stale, which keeps the
// behaviour from before maxAge existed.
func (db *geoIP2DB) cloudCopyStale() bool {
	key := fmt.Sprintf("%s.mmdb", db.edition)
	exists, err := db.cloudStorage.Exists(key)
	if err != nil || !exists {
		return false
	}
	stored, err := db.cloudStorage.GetLastModified(key)
	if err != nil {
		log.Warnf("Failed to get cloud copy time: %s", err.Error())
		return false
	}
	if age := db.now().Sub(stored); age > db.maxAge {
		log.Infof("Cloud copy was stored %s, older than %s; checking MaxMind", stored.Format(time.RFC3339), db.maxAge)
		return true
	}
	return false
}

// openFromCloud loads and opens the cloud copy, reporting whether a new
// database is now in service.
func (db *geoIP2DB) openFromCloud() bool {
	f, err := db.loadFromCloudStorage()
	if err != nil {
		log.Warnf("Failed to load from cloud storage: %s", err.Error())
		return false
	}
	if f.path == "" {
		return false
	}
	if err := db.use(f); err != nil {
		os.Remove(f.path)
		return false
	}
	return true
}

// fetched is a database file on disk and what identifies its version.
type fetched struct {
	path    string
	etag    string
	modTime time.Time
}

// use opens f and, only once it is in service, records its ETag and time.
func (db *geoIP2DB) use(f fetched) error {
	if err := db.openDatabase(f.path); err != nil {
		return err
	}
	db.etag = f.etag
	// Queries read modTime under the lock.
	db.Lock()
	db.modTime = f.modTime
	db.Unlock()
	return nil
}

func (db *geoIP2DB) hasReader() bool {
	db.Lock()
	defer db.Unlock()
	return db.reader != nil
}

func (db *geoIP2DB) openDatabase(path string) error {
	log.Infof("Opening DB: %s", path)
	reader, err := geoip2.Open(path)
	if err != nil {
		log.Errorf("Failed to open GeoIP2: %s", err.Error())
		return err
	}
	db.Lock()
	if db.reader != nil {
		log.Infof("Closing outdated DB")
		db.reader.Close()
	}
	db.reader = reader
	db.Unlock()
	if db.path != "" {
		log.Infof("Deleting outdated DB: %s", db.path)
		os.Remove(db.path)
	}
	db.path = path
	return nil
}

// loadFromCloudStorage fetches the cloud copy into a temp file. It leaves
// db.etag and db.modTime alone: they describe the database in service, and
// the caller sets them once this file has opened.
func (db *geoIP2DB) loadFromCloudStorage() (fetched, error) {
	key := fmt.Sprintf("%s.mmdb", db.edition)

	// Check if database exists in cloud storage
	exists, err := db.cloudStorage.Exists(key)
	if err != nil {
		return fetched{}, fmt.Errorf("failed to check cloud storage: %w", err)
	}
	if !exists {
		log.Infof("Database not found in cloud storage: %s", key)
		return fetched{}, nil
	}

	// Get ETag from cloud storage metadata
	metadata, err := db.cloudStorage.GetMetadata(key)
	if err != nil {
		return fetched{}, fmt.Errorf("failed to get metadata from cloud storage: %w", err)
	}

	cloudETag := metadata["etag"]
	if cloudETag != "" {
		// Check if ETag has changed since last load
		if db.etag == cloudETag {
			log.Infof("Cloud storage ETag unchanged: %s - skipping download", cloudETag)
			return fetched{}, nil // No download needed
		}

		log.Infof("Found new ETag in cloud storage: %s (previous: %s)", cloudETag, db.etag)
	}

	// Download database from cloud storage
	reader, err := db.cloudStorage.Download(key)
	if err != nil {
		return fetched{}, fmt.Errorf("failed to download from cloud storage: %w", err)
	}
	defer reader.Close()

	// Create temporary file
	outfile, err := os.CreateTemp("", db.edition)
	if err != nil {
		return fetched{}, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outfile.Close()

	// Copy data to temporary file
	_, err = io.Copy(outfile, reader)
	if err != nil {
		os.Remove(outfile.Name())
		return fetched{}, fmt.Errorf("failed to copy data from cloud storage: %w", err)
	}

	// Get modification time
	modTime, err := db.cloudStorage.GetLastModified(key)
	if err != nil {
		log.Warnf("Failed to get modification time from cloud storage: %s", err.Error())
		modTime = time.Now()
	}

	log.Infof("Successfully loaded database from cloud storage: %s", outfile.Name())
	return fetched{path: outfile.Name(), etag: cloudETag, modTime: modTime}, nil
}

// download fetches a newer database from MaxMind into a temp file, or returns
// an empty path when MaxMind reports no change. Like loadFromCloudStorage it
// leaves db.etag and db.modTime to the caller.
func (db *geoIP2DB) download() (fetched, error) {
	u := fmt.Sprintf(db.downloadURL, db.edition, db.licenseKey)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		// The parse error would quote the URL, license key included.
		log.Errorf("Failed to create download request for %s", db.edition)
		return fetched{}, fmt.Errorf("download %s: invalid request URL", db.edition)
	}
	if db.etag != "" {
		req.Header.Set("If-None-Match", db.etag)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		// A *url.Error quotes the URL, and the URL carries the license key.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fetched{}, fmt.Errorf("download %s: %w", db.edition, err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 200:
	case 304:
		log.Infof("Not modified: %s => %s", db.edition, db.etag)
		return fetched{}, nil
	default:
		log.Errorf("Failed to download %s: %s", db.edition, res.Status)
		return fetched{}, errors.New(res.Status)
	}
	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		log.Errorf("Failed to read gzip stream: %s", err.Error())
		return fetched{}, err
	}
	filename := fmt.Sprintf("%s.mmdb", db.edition)
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Errorf("Failed to iterate tar stream: %s", err.Error())
			return fetched{}, err
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if strings.HasSuffix(header.Name, filename) {
				outfile, err := os.CreateTemp("", db.edition)
				if err != nil {
					log.Errorf("Failed to create temp file: %s", err.Error())
					return fetched{}, err
				}
				defer outfile.Close()
				log.Infof("Downloading DB: %s => %d bytes", filename, header.Size)
				_, err = io.CopyN(outfile, tr, header.Size)
				if err != nil {
					os.Remove(outfile.Name())
					log.Errorf("Failed to copy tar stream: %s", err.Error())
					return fetched{}, err
				}
				return fetched{path: outfile.Name(), etag: res.Header.Get("Etag"), modTime: header.ModTime}, nil
			} else {
				_, err := io.CopyN(io.Discard, tr, header.Size)
				if err != nil {
					log.Errorf("Failed to drain tar stream: %s", err.Error())
					return fetched{}, err
				}
			}
		default:
			log.Warnf("Unknown type in tar stream: %v in %s", header.Typeflag, header.Name)
		}
	}
	log.Errorf("Not found: %s", filename)
	return fetched{}, fmt.Errorf("not found: %s", filename)
}

func (db *geoIP2DB) storeInCloudStorage(localPath string) error {
	key := fmt.Sprintf("%s.mmdb", db.edition)

	// Open the local file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	// Prepare metadata with ETag
	metadata := map[string]string{
		"etag":          db.etag,
		"edition":       db.edition,
		"download_time": time.Now().Format(time.RFC3339),
	}

	// Upload to cloud storage
	log.Infof("Storing database in cloud storage: %s", key)
	err = db.cloudStorage.UploadWithMetadata(key, file, metadata)
	if err != nil {
		return fmt.Errorf("failed to upload to cloud storage: %w", err)
	}

	log.Infof("Successfully stored database in cloud storage with ETag: %s", db.etag)
	return nil
}
