package dns

import (
	"fmt"
	"net"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/crosstalkio/log"
)

func detectExternalIP(s log.Sugar) (net.IP, error) {
	s.Debugf("Detecting external IP...")
	if metadata.OnGCE() {
		str, err := metadata.ExternalIP()
		if err == nil {
			ip := net.ParseIP(str)
			if ip == nil {
				s.Errorf("Invalid GCE external IP: %s", str)
				return nil, fmt.Errorf("Invalid GCE external IP: %s", str)
			}
			return ip, nil
		}
	}
	detector := NewConcurrentIPDetector(s,
		&HTTPDetector{
			Sugar:   s,
			URL:     "https://api.ipify.org",
			Timeout: 10 * time.Second,
		},
		&HTTPDetector{
			Sugar:   s,
			URL:     "https://icanhazip.com",
			Timeout: 10 * time.Second,
		},
	)
	ip, err := detector.Detect()
	if err != nil {
		s.Errorf("Failed to detect external IP: %s", err.Error())
		return nil, err
	}
	return ip, nil
}
