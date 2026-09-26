package controller

import (
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
	cacheKey := cache.IPKey("city", ip)
	var city db.City
	err := cache.Unmarshal(cacheKey, &city)
	if err == nil {
		log.Debugf("Hit city location cache")
		city.IP = ip.String()
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
		err = cache.Marshal(cacheKey, withoutCityIP(city))
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
	cacheKey := cache.IPKey("country", ip)
	var country db.Country
	err := cache.Unmarshal(cacheKey, &country)
	if err == nil {
		log.Debugf("Hit country location cache")
		country.IP = ip.String()
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
		err = cache.Marshal(cacheKey, withoutCountryIP(country))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, r, country)
		return
	}
}

// The cached value leaves the IP out as the key does; a hit puts it back.

func withoutCityIP(c *db.City) *db.City {
	out := *c
	out.IP = ""
	return &out
}

func withoutCountryIP(c *db.Country) *db.Country {
	out := *c
	out.IP = ""
	return &out
}
