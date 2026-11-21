package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/servers"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
func run() error {
	var cfg conf.Config
	var chunkKB int64
	var debug bool
	flag.StringVar(&cfg.SyncFolder, "sync-folder", "", "Folder to synchronize (required)")
	flag.StringVar(&cfg.StateDir, "state-dir", "./badgerdb-v2", "Persistent state outside the synchronized folder")
	flag.Int64Var(&chunkKB, "chunk-size", 64, "Transfer chunk size in KiB (1–1024)")
	flag.DurationVar(&cfg.SyncInterval, "sync-interval", time.Second, "Reconciliation and scan interval")
	flag.StringVar(&cfg.Port, "port", "50051", "Listening port")
	flag.StringVar(&cfg.Peers, "peers", "", "Optional comma-separated peer host:port addresses")
	flag.BoolVar(&cfg.NoMDNS, "no-mdns", false, "Disable multicast discovery")
	flag.BoolVar(&cfg.Insecure, "insecure", false, "Explicitly allow unauthenticated plaintext (isolated tests only)")
	flag.StringVar(&cfg.TLSCert, "tls-cert", "", "PEM certificate for this peer")
	flag.StringVar(&cfg.TLSKey, "tls-key", "", "PEM private key for this peer")
	flag.StringVar(&cfg.TLSCA, "tls-ca", "", "PEM CA trusted for peer certificates")
	flag.BoolVar(&debug, "debug", false, "Enable debug logs")
	flag.Parse()
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	if cfg.SyncFolder == "" {
		return fmt.Errorf("--sync-folder is required")
	}
	if chunkKB < 1 || chunkKB > 1024 {
		return fmt.Errorf("--chunk-size must be 1–1024 KiB")
	}
	cfg.ChunkSize = chunkKB * 1024
	if cfg.SyncInterval <= 0 {
		return fmt.Errorf("--sync-interval must be positive")
	}
	port, err := servers.ParsePort(cfg.Port)
	if err != nil {
		return err
	}
	serverCreds, clientCreds, err := transport(cfg)
	if err != nil {
		return err
	}
	for _, p := range strings.Split(cfg.Peers, ",") {
		if p != "" {
			if _, _, err = net.SplitHostPort(p); err != nil {
				return fmt.Errorf("invalid peer %q: %w", p, err)
			}
		}
	}
	folder, err := filepath.Abs(cfg.SyncFolder)
	if err != nil {
		return err
	}
	engine, err := servers.NewEngine(folder, cfg.StateDir)
	if err != nil {
		return err
	}
	defer engine.Close()
	engine.BlockSize = int(cfg.ChunkSize)
	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer(grpc.Creds(serverCreds), grpc.WaitForHandlers(true))
	pb.RegisterFileSyncServiceServer(grpcServer, &servers.Service{Engine: engine, ChunkSize: int(cfg.ChunkSize)})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := grpcServer.Serve(listener); err != nil {
			log.Error(err)
			cancel()
		}
	}()
	connections := servers.NewConnections(ctx, engine, cfg.SyncInterval, grpc.WithTransportCredentials(clientCreds))
	var stopDiscovery func()
	if !cfg.NoMDNS {
		stopDiscovery, err = servers.Discover(ctx, &wg, engine.ID(), port, connections)
		if err != nil {
			cancel()
			connections.Close()
			grpcServer.Stop()
			wg.Wait()
			return err
		}
	}
	for _, p := range strings.Split(cfg.Peers, ",") {
		if p != "" {
			connections.AddPeer(p)
		}
	}
	wg.Add(1)
	go func() { defer wg.Done(); engine.Watch(ctx, cfg.SyncInterval) }()
	log.Infof("Syncing %s on port %s", folder, cfg.Port)
	<-ctx.Done()
	if stopDiscovery != nil {
		stopDiscovery()
	}
	connections.Close()
	grpcServer.Stop()
	wg.Wait()
	return nil
}
func transport(cfg conf.Config) (credentials.TransportCredentials, credentials.TransportCredentials, error) {
	if cfg.Insecure {
		if cfg.TLSCert != "" || cfg.TLSKey != "" || cfg.TLSCA != "" {
			return nil, nil, fmt.Errorf("--insecure cannot be combined with TLS flags")
		}
		return insecure.NewCredentials(), insecure.NewCredentials(), nil
	}
	if cfg.TLSCert == "" || cfg.TLSKey == "" || cfg.TLSCA == "" {
		return nil, nil, fmt.Errorf("provide --tls-cert, --tls-key and --tls-ca, or explicitly use --insecure for isolated tests")
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		return nil, nil, err
	}
	pem, err := os.ReadFile(cfg.TLSCA)
	if err != nil {
		return nil, nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, nil, fmt.Errorf("invalid CA certificate")
	}
	server := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
	// Certificates use a common service DNS SAN. Trust is restricted to the
	// explicitly configured private CA, not unauthenticated mDNS TXT records.
	client := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: pool, ServerName: "gosync"}
	return credentials.NewTLS(server), credentials.NewTLS(client), nil
}
