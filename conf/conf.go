package conf

import "time"

type Config struct {
	SyncFolder             string
	StateDir               string
	ChunkSize              int64
	SyncInterval           time.Duration
	Port                   string
	Peers                  string
	NoMDNS                 bool
	Insecure               bool
	TLSCert, TLSKey, TLSCA string
}
