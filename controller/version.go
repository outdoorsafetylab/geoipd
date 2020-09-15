package controller

import (
	"geoipd/api"
	"geoipd/model"
	"time"

	"github.com/crosstalkio/rest"
)

type ConfigController struct {
	*model.Version
}

func (c *ConfigController) Get(s *rest.Session) {
	res := &api.GetVersionResponse{
		Time: &api.GetVersionResponse_Time{
			Epoch:   c.Time.Unix(),
			Rfc3339: c.Time.Format(time.RFC3339),
		},
		Commit: c.Commit,
		Tag:    c.Tag,
	}
	s.Status(200, res)
}
