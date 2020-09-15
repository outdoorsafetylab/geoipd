package server

import (
	"geoipd/config"
	"geoipd/controller"
	"geoipd/middleware"
	"geoipd/model"

	"github.com/crosstalkio/log"
	"github.com/crosstalkio/rest"
	"github.com/gorilla/mux"
)

func NewRouter(logger log.Logger, ver *model.Version) *mux.Router {
	cfg := config.Get()
	rest := rest.NewServer(logger)
	rest.Use(middleware.Dump)
	rest.Use(middleware.NoCache)

	r := mux.NewRouter()

	endpoint := r.PathPrefix(cfg.GetString("endpoint")).Subrouter()

	config := &controller.ConfigController{
		Version: ver,
	}
	endpoint.Methods("GET").Path("/version").Handler(rest.HandlerFunc(config.Get))

	geoip := &controller.GeoIPController{}
	endpoint.Methods("GET").Path("/city").Handler(rest.HandlerFunc(geoip.City))
	endpoint.Methods("GET").Path("/country").Handler(rest.HandlerFunc(geoip.Country))

	return r
}
