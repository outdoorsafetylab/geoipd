package dns

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/crosstalkio/log"
)

type HTTPDetector struct {
	log.Sugar
	URL     string
	Header  http.Header
	Timeout time.Duration
}

func (d *HTTPDetector) Name() string {
	return d.URL
}

func (d *HTTPDetector) Detect() (net.IP, error) {
	d.Debugf("Detecting external IP: %s", d.URL)
	req, err := http.NewRequest("GET", d.URL, nil)
	if err != nil {
		d.Errorf("Failed to create request '%s': %s", d.URL, err.Error())
		return nil, err
	}
	if d.Header != nil {
		for k, vals := range d.Header {
			req.Header[k] = vals
		}
	}
	client := http.Client{Timeout: d.Timeout}
	res, err := client.Do(req)
	if err != nil {
		d.Errorf("Failed to make request '%s': %s", d.URL, err.Error())
		return nil, err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		d.Errorf("Failed to read response '%s': %s", d.URL, err.Error())
		return nil, err
	}
	addr := strings.TrimSpace(string(data))
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, fmt.Errorf("Invalid IP '%s': %s", d.URL, addr)
	}
	return ip, nil
}
