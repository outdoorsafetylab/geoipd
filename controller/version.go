package controller

import (
	"net/http"
	"service/version"
	"time"
)

type ConfigController struct{}

func (c *ConfigController) GetVersion(w http.ResponseWriter, r *http.Request) {
	res := &struct {
		Time   time.Time `json:"time"`
		Commit string    `json:"commit"`
		Tag    string    `json:"tag"`
	}{
		Time:   version.Time(),
		Commit: version.GitHash,
		Tag:    version.GitTag,
	}
	writeJSON(w, r, res)
}
