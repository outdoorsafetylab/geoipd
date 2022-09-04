package controller

import (
	"net/http"
	"service/version"
)

type ConfigController struct{}

func (c *ConfigController) GetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, version.Get())
}
