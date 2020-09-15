package main

import (
	"flag"
	"fmt"
	"geoipd/cache"
	"geoipd/db"
	"geoipd/model"
	"geoipd/server"
	"os"
	"strconv"
	"time"

	"geoipd/config"
	"geoipd/log"
)

var (
	GitTime string
	GitHash string
	GitTag  string
)

func main() {
	name := flag.String("c", "config", "")
	flag.Usage = func() {
		fmt.Printf("Usage: %s -c <config name>\n", os.Args[0])
		os.Exit(1)
	}
	flag.Parse()
	if err := config.Init(*name); err != nil {
		os.Exit(1)
	}
	err := log.Init(config.Get().GetBool("dev"))
	if err != nil {
		os.Exit(-1)
	}
	s := log.GetSugar()
	err = cache.Init(s)
	if err != nil {
		os.Exit(-1)
	}
	defer cache.Deinit(s)
	err = db.Init(s)
	if err != nil {
		os.Exit(-1)
	}
	defer db.Deinit(s)
	server := server.New(s)
	t, _ := strconv.ParseInt(GitTime, 10, 64)
	if t <= 0 {
		t = time.Now().Unix()
	}
	ver := &model.Version{
		Time:   time.Unix(t, 0),
		Commit: GitHash,
		Tag:    GitTag,
	}
	err = server.Run(ver)
	if err == nil {
		os.Exit(0)
	} else {
		os.Exit(-1)
	}
}
