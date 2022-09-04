package cache

import (
	"encoding/json"
	"fmt"
	"time"

	"service/config"
	"service/log"

	"github.com/go-redis/redis/v7"
)

const Miss = Error("cache: miss")

type Error string

func (e Error) Error() string { return string(e) }

var client *redis.Client
var expire time.Duration

func Init() error {
	cfg := config.Get()
	var err error
	redisOpts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.GetString("redis.host"), cfg.GetInt("redis.port")),
		Password: cfg.GetString("redis.pass"),
	}
	client = redis.NewClient(redisOpts)
	_, err = client.Ping().Result()
	if err != nil {
		log.Errorf("Failed to ping redis: %s", err.Error())
		return err
	}
	value := cfg.GetString("redis.expire")
	expire, err = time.ParseDuration(value)
	if err != nil {
		log.Errorf("Invalid expire duration: %s", value)
		return err
	}
	return nil
}

func Deinit() {
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

func Unmarshal(key string, val interface{}) error {
	data, err := Get(key)
	if err != nil {
		return err
	}
	if data == nil {
		return Miss
	}
	return json.Unmarshal(data, val)
}

func Marshal(key string, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return Set(key, data)
}
