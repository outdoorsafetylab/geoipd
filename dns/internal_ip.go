package dns

import (
	"fmt"
	"net"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"github.com/crosstalkio/log"
)

func detectInternalIP(s log.Sugar) (net.IP, error) {
	s.Debugf("Detecting internal IP...")
	if metadata.OnGCE() {
		str, err := metadata.InternalIP()
		if err == nil {
			ip := net.ParseIP(str)
			if ip == nil {
				s.Errorf("Invalid GCE internal IP: %s", str)
				return nil, fmt.Errorf("Invalid GCE internal IP: %s", str)
			}
			return ip, nil
		}
	}
	ifs, err := net.Interfaces()
	if err != nil {
		s.Errorf("Failed to list interfaces: %s", err.Error())
		return nil, err
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			s.Errorf("Failed to get interface addr: %s", err.Error())
			return nil, err
		}
		if len(addrs) <= 0 {
			continue
		}
		prefixes := []string{"eth", "en", "wl", ""}
		for _, prefix := range prefixes {
			if strings.HasPrefix(i.Name, prefix) {
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok {
						if ipnet.IP.To4() != nil {
							s.Debugf("Using interface IP of '%s': %v", i.Name, ipnet.IP)
							return ipnet.IP, nil
						}
					}
				}
			}
		}
	}
	s.Warningf("No interface addr was found")
	return nil, nil
}
