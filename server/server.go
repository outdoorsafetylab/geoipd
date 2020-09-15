package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"geoipd/config"
	"geoipd/model"

	"path/filepath"

	"github.com/crosstalkio/log"
	"golang.org/x/crypto/acme/autocert"
)

type server struct {
	log.Sugar
	signal  chan os.Signal
	httpErr chan error
}

func New(log log.Sugar) *server {
	s := &server{
		Sugar:   log,
		httpErr: make(chan error, 1),
		signal:  make(chan os.Signal, 1),
	}
	return s
}

func (s *server) Run(ver *model.Version) error {
	r := NewRouter(s, ver)
	http.Handle("/", r)
	go func() {
		s.httpErr <- s.bindHTTP()
	}()
	signal.Notify(s.signal, os.Interrupt)
	for {
		select {
		case err := <-s.httpErr:
			if err != nil {
				s.Errorf("HTTP error: %s", err.Error())
				return err
			}
		case <-s.signal:
			s.Infof("Interrupted")
			return nil
		}
	}
}

func (s *server) bindHTTP() error {
	cfg := config.Get()
	port := cfg.GetInt("port")
	addr := fmt.Sprintf(":%d", port)
	cert := cfg.GetString("cert.type")
	switch cert {
	case "", "off", "none":
		s.Infof("Listening HTTP on port %d", port)
		return http.ListenAndServe(addr, nil)
	case "file":
		crtFile := cfg.GetString("cert.crtfile")
		keyFile := cfg.GetString("cert.keyfile")
		if crtFile == "" || keyFile == "" {
			return fmt.Errorf("Missing 'cert.crtfile' or 'cert.keyfile' config")
		}
		s.Infof("Listening HTTPS with '%s' and '%s' on port %d", crtFile, keyFile, port)
		return http.ListenAndServeTLS(addr, crtFile, keyFile, nil)
	case "auto":
		m := &autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Email:  cfg.GetString("cert.email"),
		}
		cacheDir := cfg.GetString("cert.cache_dir")
		if cacheDir != "" {
			dir, err := filepath.Abs(cacheDir)
			if err != nil {
				return err
			}
			m.Cache = autocert.DirCache(dir)
		}
		hostname := cfg.GetString("cert.host")
		var err error
		if hostname == "" {
			hostname, err = os.Hostname()
			if err != nil {
				return err
			}
		}
		domain := cfg.GetString("cert.domain")
		if domain == "" {
			m.HostPolicy = func(_ context.Context, host string) error {
				if host != hostname {
					return fmt.Errorf("Hostname mismatch: expect=%q, was=%q", hostname, host)
				}
				return nil
			}
		} else {
			m.HostPolicy = func(c context.Context, host string) error {
				if !strings.HasSuffix(host, domain) {
					s.Warningf("Not expected domain: %s", host)
					return fmt.Errorf("Not allowed")
				}
				return nil
			}
		}
		config := m.TLSConfig()
		getCert := config.GetCertificate
		config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if hello.ServerName == "" {
				hello.ServerName = hostname
			}
			return getCert(hello)
		}
		srv := &http.Server{
			Addr:      addr,
			TLSConfig: config,
		}
		s.Infof("Listening HTTPS with auto-certs on port %d", port)
		return srv.ListenAndServeTLS("", "")
	default:
		return fmt.Errorf("Unexpected 'cert.type' config: %s", cert)
	}
}
