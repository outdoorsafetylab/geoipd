package version

import (
	"strconv"
	"time"
)

var (
	BuildTime string
	GitHash   string
	GitTag    string
)

func Time() time.Time {
	var t time.Time
	v, err := strconv.ParseInt(BuildTime, 10, 64)
	if err == nil {
		t = time.Unix(v, 0)
	}
	return t
}
