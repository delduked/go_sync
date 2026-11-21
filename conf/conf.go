package conf

import "time"

type Config struct {
	Bootstrap              string
	SyncFolder             string
	StateDir               string
	ChunkSize              int64
	SyncInterval           time.Duration
	RepairInterval         time.Duration
	Port                   string
	Peers                  string
	NoMDNS                 bool
	Insecure               bool
	TLSCert, TLSKey, TLSCA string
}
