package conf

import "time"

type Config struct {
	SyncFolder   string
	ChunkSize    int64
	SyncInterval time.Duration
	Port         string
}

var AppConfig Config
