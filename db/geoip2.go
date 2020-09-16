package db

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"geoipd/config"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crosstalkio/log"
	"github.com/oschwald/geoip2-golang"
)

var db *geoIP2DB
var ticker *time.Ticker
var done chan bool

func Init(s log.Sugar) error {
	cfg := config.Get()
	key := cfg.GetString("geoip2.license_key")
	if key == "" {
		s.Errorf("Please specify 'geoip2.license_key' in YAML config or set GEOIP2_LICENSE_KEY environment variable in order to download DB.")
		return errors.New("Missing license key")
	}
	edition := cfg.GetString("geoip2.edition")
	db = newGeoIP2DB(key, edition)
	err := db.renew(s)
	if err != nil {
		return err
	}
	renew := cfg.GetString("geoip2.renew")
	if renew != "" {
		du, err := time.ParseDuration(renew)
		if err != nil {
			s.Errorf("Invalid renew duration: %s", renew)
			return err
		}
		s.Infof("Scheduling DB renew every %s", renew)
		ticker = time.NewTicker(du)
		done = make(chan bool)
		go func() {
			for {
				select {
				case <-done:
					return
				case t := <-ticker.C:
					s.Infof("Renewing DB at %s", t.String())
					_ = db.renew(s)
				}
			}
		}()
	}
	return nil
}

func Deinit(s log.Sugar) {
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
	licenseKey string
	edition    string
	etag       string
	modTime    time.Time
	path       string
	reader     *geoip2.Reader
}

func newGeoIP2DB(licenseKey, edition string) *geoIP2DB {
	return &geoIP2DB{
		licenseKey: licenseKey,
		edition:    edition,
	}
}

func (db *geoIP2DB) renew(s log.Sugar) error {
	path, err := db.download(s)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	s.Infof("Opening DB: %s", path)
	reader, err := geoip2.Open(path)
	if err != nil {
		s.Fatalf("Failed to open GeoIP2: %s", err.Error())
		return err
	}
	db.Lock()
	if db.reader != nil {
		s.Infof("Closing outdated DB")
		db.reader.Close()
	}
	db.reader = reader
	db.Unlock()
	if db.path != "" {
		s.Infof("Deleting outdated DB: %s", db.path)
		os.Remove(db.path)
	}
	db.path = path
	return nil
}

func (db *geoIP2DB) download(s log.Sugar) (string, error) {
	url := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&suffix=tar.gz", db.edition, db.licenseKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		s.Errorf("Failed to create request: %s", err.Error())
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
		s.Infof("Not modified: %s => %s", db.edition, db.etag)
		return "", nil
	default:
		s.Errorf("Failed to download %s: %s", db.edition, res.Status)
		return "", errors.New(res.Status)
	}
	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		s.Errorf("Failed to read gzip stream: %s", err.Error())
		return "", err
	}
	filename := fmt.Sprintf("%s.mmdb", db.edition)
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			s.Errorf("Failed to iterate tar stream: %s", err.Error())
			return "", err
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if strings.HasSuffix(header.Name, filename) {
				outfile, err := ioutil.TempFile("", db.edition)
				if err != nil {
					s.Errorf("Failed to create temp file: %s", err.Error())
					return "", err
				}
				defer outfile.Close()
				s.Infof("Getting DB: %s => %d bytes", filename, header.Size)
				_, err = io.CopyN(outfile, tr, header.Size)
				if err != nil {
					s.Errorf("Failed to copy tar stream: %s", err.Error())
					return "", err
				}
				db.etag = res.Header.Get("Etag")
				db.modTime = header.ModTime
				s.Infof("Updating etag: %s => %s", filename, db.etag)
				return outfile.Name(), nil
			} else {
				_, err := io.CopyN(ioutil.Discard, tr, header.Size)
				if err != nil {
					s.Errorf("Failed to drain tar stream: %s", err.Error())
					return "", err
				}
			}
		default:
			s.Warningf("Unknown type in tar stream: %s in %s", header.Typeflag, header.Name)
		}
	}
	s.Errorf("Not found: %s", filename)
	return "", fmt.Errorf("Not found: %s", filename)
}
