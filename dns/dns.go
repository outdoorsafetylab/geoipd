package dns

import (
	"errors"
	"net"
	"time"

	"service/config"

	"cloud.google.com/go/compute/metadata"
	"github.com/crosstalkio/godaddy"
	"github.com/crosstalkio/log"
)

func Init(s log.Sugar) error {
	cfg := config.Get()
	ttl := cfg.GetInt("dns.ttl")
	if ttl <= 0 {
		return nil
	}
	host, err := GetHost()
	if err != nil {
		return err
	}
	domain, err := GetDomain()
	if err != nil {
		return err
	}
	ip, err := detectExternalIP(s)
	if err != nil {
		return err
	}
	kind := "A"
	if ip.To4() == nil {
		kind = "AAAA"
	}
	addr := ip.String()
	s.Infof("Updating DNS: %s.%s => %s (TTL: %d)", host, domain, addr, ttl)
	client := godaddy.NewClient(cfg.GetString("godaddy.url"), cfg.GetString("godaddy.key"), cfg.GetString("godaddy.secret"), time.Second*time.Duration(cfg.GetInt64("godaddy.timeout")))
	err = client.PutRecord(domain, kind, host, addr, ttl)
	if err != nil {
		s.Errorf("Failed to update DNS: %s", err.Error())
		return err
	}
	return nil
}

func GetInternalIP(s log.Sugar) (net.IP, error) {
	return detectInternalIP(s)
}

func GetExternalIP(s log.Sugar) (net.IP, error) {
	return detectExternalIP(s)
}

func GetHost() (string, error) {
	cfg := config.Get()
	host := cfg.GetString("dns.host")
	if host == "" {
		if metadata.OnGCE() {
			host, err := metadata.InstanceName()
			if err == nil {
				return host, nil
			}
		}
		return "", errors.New("Failed to resolve host")
	}
	return host, nil
}

func GetDomain() (string, error) {
	cfg := config.Get()
	domain := cfg.GetString("dns.domain")
	if domain == "" {
		return "", errors.New("Failed to resolve domain")
	}
	return domain, nil
}
