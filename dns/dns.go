package dns

import (
	"fmt"
	"net"
	"time"

	"geoipd/config"

	"cloud.google.com/go/compute/metadata"
	"github.com/crosstalkio/godaddy"
	"github.com/crosstalkio/log"
)

var internalIP net.IP
var externalIP net.IP

func Init(s log.Sugar) error {
	var err error
	internalIP, err = detectInternalIP(s)
	if err != nil {
		return err
	}
	externalIP, err = detectExternalIP(s)
	if err != nil {
		return err
	}
	cfg := config.Get()
	host := cfg.GetString("dns.host")
	if host == "" && metadata.OnGCE() {
		host, err = metadata.InstanceName()
		if err == nil {
			cfg.Set("dns.host", host)
			s.Infof("Using GCE instance name for 'dns.host': %s", host)
		}
	}
	domain := cfg.GetString("dns.domain")
	if domain == "" {
		return nil
	}
	ttl := cfg.GetInt("dns.ttl")
	if ttl <= 0 {
		return fmt.Errorf("Invalid or missing 'dns.ttl' config: %d", ttl)
	}
	kind := "A"
	if externalIP.To4() == nil {
		kind = "AAAA"
	}
	addr := externalIP.String()
	s.Infof("Updating DNS: %s.%s => %s (TTL: %d)", host, domain, addr, ttl)
	client := godaddy.NewClient(cfg.GetString("godaddy.url"), cfg.GetString("godaddy.key"), cfg.GetString("godaddy.secret"), time.Second*time.Duration(cfg.GetInt64("godaddy.timeout")))
	err = client.PutRecord(domain, kind, host, addr, ttl)
	if err != nil {
		s.Errorf("Failed to update DNS: %s", err.Error())
		return err
	}
	return nil
}

func GetInternalIP() net.IP {
	return internalIP
}

func GetExternalIP() net.IP {
	return externalIP
}
