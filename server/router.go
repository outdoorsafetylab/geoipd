package server

import (
	"net/http"
	"service/config"
	"service/controller"
	"service/middleware"

	"github.com/gorilla/mux"
)

func NewRouter(root http.FileSystem) *mux.Router {
	cfg := config.Get()

	r := mux.NewRouter()

	endpoint := r.PathPrefix(cfg.GetString("endpoint")).Subrouter()
	endpoint.Use(middleware.Dump)
	endpoint.Use(middleware.NoCache)

	config := &controller.ConfigController{}
	endpoint.HandleFunc("/version", config.GetVersion).Methods("GET")

	geoip := &controller.GeoIPController{}
	endpoint.HandleFunc("/city", geoip.City).Methods("GET")
	endpoint.HandleFunc("/country", geoip.Country).Methods("GET")

	if root != nil {
		r.NotFoundHandler = http.FileServer(root)
	}
	return r
}
