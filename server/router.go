package server

import (
	"net/http"
	"service/config"
	"service/controller"
	"service/middleware"

	"github.com/crosstalkio/log"
	"github.com/crosstalkio/rest"
	"github.com/gorilla/mux"
)

func NewRouter(s log.Logger, root http.FileSystem) *mux.Router {
	cfg := config.Get()
	rest := rest.NewServer(s)
	rest.Use(middleware.Dump)
	rest.Use(middleware.NoCache)

	r := mux.NewRouter()

	endpoint := r.PathPrefix(cfg.GetString("endpoint")).Subrouter()

	config := &controller.ConfigController{}
	endpoint.Methods("GET").Path("/version").Handler(rest.HandlerFunc(config.Get))

	geoip := &controller.GeoIPController{}
	endpoint.Methods("GET").Path("/city").Handler(rest.HandlerFunc(geoip.City))
	endpoint.Methods("GET").Path("/country").Handler(rest.HandlerFunc(geoip.Country))

	if root != nil {
		r.NotFoundHandler = http.FileServer(root)
	}
	return r
}
