package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"

	"service/config"
	"service/dns"

	"github.com/crosstalkio/httpd"
	"github.com/crosstalkio/log"
)

type server struct {
	log.Sugar
	signal      chan os.Signal
	httpErr     chan error
	redirectErr chan error
}

func New(s log.Sugar) *server {
	server := &server{
		Sugar:       s,
		signal:      make(chan os.Signal, 1),
		httpErr:     make(chan error, 1),
		redirectErr: make(chan error, 1),
	}
	return server
}

func (s *server) Run(root http.FileSystem) error {
	cfg := config.Get()
	var tls *tls.Config
	var err error
	cert := cfg.GetString("cert.type")
	switch cert {
	case "", "off", "none":
	case "file":
		keyFile := cfg.GetString("cert.keyfile")
		crtFile := cfg.GetString("cert.crtfile")
		if crtFile == "" || keyFile == "" {
			return fmt.Errorf("Missing 'cert.keyfile' or 'cert.crtfile' config")
		}
		tls, err = httpd.GetCertFileConfig(s, keyFile, crtFile)
		if err != nil {
			return err
		}
	case "host":
		host, err := dns.GetHost()
		if err != nil {
			return err
		}
		domain, err := dns.GetDomain()
		if err != nil {
			return err
		}
		email := cfg.GetString("cert.email")
		if email == "" {
			return fmt.Errorf("Missing 'cert.email' config")
		}
		cacheDir := cfg.GetString("cert.cache_dir")
		tls, err = httpd.GetAutoHostCertConfig(s, fmt.Sprintf("%s.%s", host, domain), email, cacheDir)
		if err != nil {
			return err
		}
	case "domain":
		domain, err := dns.GetDomain()
		if err != nil {
			return err
		}
		email := cfg.GetString("cert.email")
		if email == "" {
			return fmt.Errorf("Missing 'cert.email' config")
		}
		cacheDir := cfg.GetString("cert.cache_dir")
		tls, err = httpd.GetAutoDomainCertConfig(s, domain, email, cacheDir)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unexpected 'cert.type' config: %s", cert)
	}
	r := NewRouter(s, root)
	go func() {
		s.httpErr <- httpd.BindHTTP(s, cfg.GetInt("port"), r, tls)
	}()
	go func() {
		port := cfg.GetInt("redirect.port")
		if port <= 0 {
			return
		}
		code := cfg.GetInt("redirect.code")
		if code <= 0 {
			code = 301
		}
		scheme := cfg.GetString("redirect.scheme")
		if scheme == "" {
			scheme = "https"
		}
		s.redirectErr <- httpd.BindHTTP(s, port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			url := fmt.Sprintf("%s://%s:%d%s", scheme, host, cfg.GetInt("port"), r.RequestURI)
			s.Debugf("Redirecting %d: %s => %s", code, r.URL, url)
			http.Redirect(w, r, url, code)
		}), nil)
	}()
	signal.Notify(s.signal, os.Interrupt)
	for {
		select {
		case err := <-s.httpErr:
			if err != nil {
				s.Errorf("HTTP error: %s", err.Error())
				return err
			}
		case err := <-s.redirectErr:
			if err != nil {
				s.Errorf("Redirect error: %s", err.Error())
				return err
			}
		case <-s.signal:
			s.Infof("Interrupted")
			return nil
		}
	}
}
