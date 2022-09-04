package controller

import (
	"fmt"
	"net"
	"net/http"
	"service/cache"
	"service/db"
	"service/log"
)

type GeoIPController struct {
}

func (c *GeoIPController) City(w http.ResponseWriter, r *http.Request) {
	remoteAddr := stringVar(r, "ip", "")
	if remoteAddr == "" {
		remoteAddr = getRemoteAddress(r)
	}
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		http.Error(w, fmt.Sprintf("Invalid IP address: %s", remoteAddr), 400)
		return
	}
	cacheKey := fmt.Sprintf("city:%s", remoteAddr)
	var city db.City
	err := cache.Unmarshal(cacheKey, &city)
	if err == nil {
		log.Infof("Hit city location cache: %s", remoteAddr)
		writeJSON(w, r, &city)
		return
	}
	if err != cache.Miss {
		http.Error(w, err.Error(), 500)
		return
	} else {
		log.Infof("Querying city location: %s", remoteAddr)
		city, err := db.QueryCity(ip)
		if err != nil {
			http.Error(w, err.Error(), 500)
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
		http.Error(w, fmt.Sprintf("Invalid IP address: %s", remoteAddr), 400)
		return
	}
	cacheKey := fmt.Sprintf("country:%s", remoteAddr)
	var country db.Country
	err := cache.Unmarshal(cacheKey, &country)
	if err == nil {
		log.Infof("Hit country location cache: %s", remoteAddr)
		writeJSON(w, r, &country)
		return
	}
	if err != cache.Miss {
		http.Error(w, err.Error(), 500)
		return
	} else {
		log.Infof("Querying country location: %s", remoteAddr)
		country, err := db.QueryCountry(ip)
		if err != nil {
			http.Error(w, err.Error(), 500)
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
