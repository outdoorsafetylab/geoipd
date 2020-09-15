package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"geoipd/cache"
	"geoipd/db"
	"net"
	"strings"

	"github.com/crosstalkio/rest"
)

type GeoIPController struct {
}

func (c *GeoIPController) City(s *rest.Session) {
	host, ip := c.getRequestIP(s)
	if ip == nil {
		s.Statusf(400, "Invalid IP address: %s", host)
		return
	}
	cacheKey := fmt.Sprintf("city:%s", host)
	data, err := cache.Get(cacheKey)
	if err != nil {
		s.Status(500, err)
		return
	}
	if data != nil {
		s.Infof("Hit city location cache: %s", host)
	} else {
		s.Infof("Querying city location: %s", host)
		city, err := db.QueryCity(ip)
		if err != nil {
			s.Status(500, err)
			return
		}
		data, err = json.Marshal(city)
		if err != nil {
			s.Status(500, err)
			return
		}
		err = cache.Set(cacheKey, data)
		if err != nil {
			s.Status(500, err)
			return
		}
	}
	c.flush(s, data)
}

func (c *GeoIPController) Country(s *rest.Session) {
	host, ip := c.getRequestIP(s)
	if ip == nil {
		s.Statusf(400, "Invalid IP address: %s", host)
		return
	}
	cacheKey := fmt.Sprintf("country:%s", host)
	data, err := cache.Get(cacheKey)
	if err != nil {
		s.Status(500, err)
		return
	}
	if data != nil {
		s.Infof("Hit country location cache: %s", host)
	} else {
		s.Infof("Querying country location: %s", host)
		country, err := db.QueryCountry(ip)
		if err != nil {
			s.Status(500, err)
			return
		}
		data, err = json.Marshal(country)
		if err != nil {
			s.Status(500, err)
			return
		}
		err = cache.Set(cacheKey, data)
		if err != nil {
			s.Status(500, err)
			return
		}
	}
	c.flush(s, data)
}

func (c *GeoIPController) getRequestIP(s *rest.Session) (string, net.IP) {
	host := s.Var("ip", "")
	if host == "" {
		ip := s.RemoteAddr()
		if ip != nil {
			host = ip.String()
		}
		return host, ip
	}
	return host, net.ParseIP(host)
}

func (c *GeoIPController) flush(s *rest.Session, data []byte) {
	if strings.Contains(s.RequestHeader().Get("User-Agent"), "Mozilla") {
		var dst bytes.Buffer
		err := json.Indent(&dst, data, "", "  ")
		if err != nil {
			s.Status(500, err)
			return
		}
		data = dst.Bytes()
	}
	s.ResponseHeader().Set("Content-Type", "application/json")
	_, _ = s.ResponseWriter.Write(data)
}
