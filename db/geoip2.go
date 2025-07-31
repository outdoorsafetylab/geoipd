package db

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
}

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

	return &geoIP2DB{
		licenseKey:   licenseKey,
		edition:      edition,
		cloudStorage: cloudStorage,
	}
}

func (db *geoIP2DB) renew() error {
	// If cloud storage is configured, try to load from there first
	if db.cloudStorage != nil {
		path, err := db.loadFromCloudStorage()
		if err != nil {
			log.Warnf("Failed to load from cloud storage: %s", err.Error())
			// Fall through to download from MaxMind
		} else if path != "" {
			// Successfully loaded from cloud storage
			return db.openDatabase(path)
		}
	}

	// Download from MaxMind (either no cloud storage or cloud storage failed/empty)
	path, err := db.download()
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	return db.openDatabase(path)
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

func (db *geoIP2DB) loadFromCloudStorage() (string, error) {
	key := fmt.Sprintf("%s.mmdb", db.edition)

	// Check if database exists in cloud storage
	exists, err := db.cloudStorage.Exists(key)
	if err != nil {
		return "", fmt.Errorf("failed to check cloud storage: %w", err)
	}
	if !exists {
		log.Infof("Database not found in cloud storage: %s", key)
		return "", nil
	}

	// Get ETag from cloud storage metadata
	metadata, err := db.cloudStorage.GetMetadata(key)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata from cloud storage: %w", err)
	}

	cloudETag := metadata["etag"]
	if cloudETag != "" {
		// Check if ETag has changed since last load
		if db.etag == cloudETag {
			log.Infof("Cloud storage ETag unchanged: %s - skipping download", cloudETag)
			return "", nil // No download needed
		}

		log.Infof("Found new ETag in cloud storage: %s (previous: %s)", cloudETag, db.etag)
		db.etag = cloudETag
	}

	// Download database from cloud storage
	reader, err := db.cloudStorage.Download(key)
	if err != nil {
		return "", fmt.Errorf("failed to download from cloud storage: %w", err)
	}
	defer reader.Close()

	// Create temporary file
	outfile, err := os.CreateTemp("", db.edition)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outfile.Close()

	// Copy data to temporary file
	_, err = io.Copy(outfile, reader)
	if err != nil {
		return "", fmt.Errorf("failed to copy data from cloud storage: %w", err)
	}

	// Get modification time
	modTime, err := db.cloudStorage.GetLastModified(key)
	if err != nil {
		log.Warnf("Failed to get modification time from cloud storage: %s", err.Error())
		modTime = time.Now()
	}
	db.modTime = modTime

	log.Infof("Successfully loaded database from cloud storage: %s", outfile.Name())
	return outfile.Name(), nil
}

func (db *geoIP2DB) download() (string, error) {
	url := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&suffix=tar.gz", db.edition, db.licenseKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Errorf("Failed to create request: %s", err.Error())
		return "", err
	}
	if db.etag != "" {
		req.Header.Set("If-None-Match", db.etag)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 200:
	case 304:
		log.Infof("Not modified: %s => %s", db.edition, db.etag)
		return "", nil
	default:
		log.Errorf("Failed to download %s: %s", db.edition, res.Status)
		return "", errors.New(res.Status)
	}
	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		log.Errorf("Failed to read gzip stream: %s", err.Error())
		return "", err
	}
	filename := fmt.Sprintf("%s.mmdb", db.edition)
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Errorf("Failed to iterate tar stream: %s", err.Error())
			return "", err
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if strings.HasSuffix(header.Name, filename) {
				outfile, err := os.CreateTemp("", db.edition)
				if err != nil {
					log.Errorf("Failed to create temp file: %s", err.Error())
					return "", err
				}
				defer outfile.Close()
				log.Infof("Downloading DB: %s => %d bytes", filename, header.Size)
				_, err = io.CopyN(outfile, tr, header.Size)
				if err != nil {
					log.Errorf("Failed to copy tar stream: %s", err.Error())
					return "", err
				}
				db.etag = res.Header.Get("Etag")
				db.modTime = header.ModTime
				log.Infof("Updating etag: %s => %s", filename, db.etag)

				// Store in cloud storage if configured
				if db.cloudStorage != nil {
					err := db.storeInCloudStorage(outfile.Name())
					if err != nil {
						log.Errorf("Failed to store in cloud storage: %s", err.Error())
						// Don't fail the download, just log the error
					}
				}

				return outfile.Name(), nil
			} else {
				_, err := io.CopyN(io.Discard, tr, header.Size)
				if err != nil {
					log.Errorf("Failed to drain tar stream: %s", err.Error())
					return "", err
				}
			}
		default:
			log.Warnf("Unknown type in tar stream: %v in %s", header.Typeflag, header.Name)
		}
	}
	log.Errorf("Not found: %s", filename)
	return "", fmt.Errorf("not found: %s", filename)
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
