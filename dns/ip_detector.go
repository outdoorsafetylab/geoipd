package dns

import (
	"fmt"
	"net"

	"github.com/crosstalkio/log"
)

type IPDetector interface {
	Name() string
	Detect() (net.IP, error)
}

type ConcurrentIPDetector struct {
	log.Sugar
	Detectors []IPDetector
}

func NewConcurrentIPDetector(log log.Sugar, detectors ...IPDetector) *ConcurrentIPDetector {
	return &ConcurrentIPDetector{Sugar: log, Detectors: detectors}
}

func (d *ConcurrentIPDetector) Name() string {
	return "Concurrent"
}

func (d *ConcurrentIPDetector) Detect() (net.IP, error) {
	chans := make([]chan net.IP, len(d.Detectors))
	for i, d := range d.Detectors {
		chans[i] = make(chan net.IP)
		go func(ch chan net.IP, d IPDetector) {
			ip, _ := d.Detect()
			ch <- ip
		}(chans[i], d)
	}
	for i, ch := range chans {
		ip := <-ch
		if ip != nil {
			d.Debugf("Using external IP detected by '%s': %s", d.Detectors[i].Name(), ip.String())
			return ip, nil
		}
	}
	return nil, fmt.Errorf("No IP detected by %d detector(s)", len(d.Detectors))
}
