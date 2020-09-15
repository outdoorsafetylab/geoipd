package cache

import (
	"fmt"
	"geoipd/config"
	"time"

	"github.com/crosstalkio/log"
	"github.com/go-redis/redis/v7"
)

var client *redis.Client
var expire time.Duration

func Init(s log.Sugar) error {
	cfg := config.Get()
	var err error
	redisOpts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.GetString("redis.host"), cfg.GetInt("redis.port")),
		Password: cfg.GetString("redis.pass"),
	}
	client = redis.NewClient(redisOpts)
	_, err = client.Ping().Result()
	if err != nil {
		s.Fatalf("Failed to ping redis: %s", err.Error())
		return err
	}
	value := cfg.GetString("redis.expire")
	expire, err = time.ParseDuration(value)
	if err != nil {
		s.Errorf("Invalid expire duration: %s", value)
		return err
	}
	return nil
}

func Deinit(s log.Sugar) {
	if client != nil {
		client.Close()
		client = nil
	}
}

func Get(key string) ([]byte, error) {
	val, err := client.Get(key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return val, err
}

func Set(key string, val []byte) error {
	return client.Set(key, val, expire).Err()
}
