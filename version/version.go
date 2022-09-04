package version

import (
	"service/model"
	"strconv"
	"time"
)

var (
	BuildTime string
	GitHash   string
	GitTag    string
)

func Get() *model.Version {
	version := &model.Version{
		Commit: GitHash,
		Tag:    GitTag,
	}
	v, err := strconv.ParseInt(BuildTime, 10, 64)
	if err == nil {
		version.Time = time.Unix(v, 0)
	}
	return version
}
