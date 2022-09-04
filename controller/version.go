package controller

import (
	"net/http"
	"service/model"
	"service/version"
)

type ConfigController struct{}

func (c *ConfigController) GetVersion(w http.ResponseWriter, r *http.Request) {
	res := &model.Version{
		Time:   version.Time(),
		Commit: version.GitHash,
		Tag:    version.GitTag,
	}
	writeJSON(w, r, res)
}
