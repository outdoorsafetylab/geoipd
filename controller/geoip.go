package controller

import (
	"fmt"
	"net"
	"net/http"
	"service/cache"
	"service/db"
	"service/log"
)

// GeoIPController never writes the queried IP to logs or error messages:
// callers may promise their users that the IP is not recorded.
type GeoIPController struct {
}

func (c *GeoIPController) City(w http.ResponseWriter, r *http.Request) {
	remoteAddr := stringVar(r, "ip", "")
	if remoteAddr == "" {
		remoteAddr = getRemoteAddress(r)
	}
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		http.Error(w, "Invalid IP address", 400)
		return
	}
	cacheKey := fmt.Sprintf("city:%s", remoteAddr)
	var city db.City
	err := cache.Unmarshal(cacheKey, &city)
	if err == nil {
		log.Debugf("Hit city location cache")
		writeJSON(w, r, &city)
		return
	}
	if err != cache.Miss {
		http.Error(w, err.Error(), 500)
		return
	} else {
		log.Debugf("Querying city location")
		city, err := db.QueryCity(ip)
		if err != nil {
			// The lookup error can quote the address.
			http.Error(w, "Failed to look up IP address", 500)
			return
		}
		err = cache.Marshal(cacheKey, city)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, r, city)
		return
	}
}

func (c *GeoIPController) Country(w http.ResponseWriter, r *http.Request) {
	remoteAddr := stringVar(r, "ip", "")
	if remoteAddr == "" {
		remoteAddr = getRemoteAddress(r)
	}
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		http.Error(w, "Invalid IP address", 400)
		return
	}
	cacheKey := fmt.Sprintf("country:%s", remoteAddr)
	var country db.Country
	err := cache.Unmarshal(cacheKey, &country)
	if err == nil {
		log.Debugf("Hit country location cache")
		writeJSON(w, r, &country)
		return
	}
	if err != cache.Miss {
		http.Error(w, err.Error(), 500)
		return
	} else {
		log.Debugf("Querying country location")
		country, err := db.QueryCountry(ip)
		if err != nil {
			// The lookup error can quote the address.
			http.Error(w, "Failed to look up IP address", 500)
			return
		}
		err = cache.Marshal(cacheKey, country)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, r, country)
		return
	}
}
