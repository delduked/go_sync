package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/servers"
	"github.com/charmbracelet/log"
	badger "github.com/dgraph-io/badger/v3"
)

// hook test
func main() {
	log.SetLevel(log.DebugLevel)
	// Parse command-line flags and initialize configurations
	parseFlags()

	// Initialize BadgerDB
	db := initDB()
	defer db.Close()

	// Initialize core services
	mdns, meta, file, conn, grpc := initServices(db)

	// Create context and waitgroup for goroutine management
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Start services
	startServices(ctx, &wg, mdns, meta, file, conn, grpc)

	// Wait for shutdown signal (e.g., CTRL+C)
	waitForShutdownSignal(cancel)

	// Wait for all goroutines to finish
	wg.Wait()

	log.Info("Application shut down gracefully.")
}

func parseFlags() {
	// Define command-line flags
	syncFolder := flag.String("sync-folder", "", "Folder to keep in sync (required)")
	chunkSizeKB := flag.Int64("chunk-size", 64, "Chunk size in kilobytes (optional)")
	syncInterval := flag.Duration("sync-interval", 1*time.Minute, "Synchronization interval (optional)")
	portNumber := flag.String("port", "50051", "Port number for the gRPC server (optional)")

	// Parse the flags
	flag.Parse()

	// Check if the required flag is provided
	if *syncFolder == "" {
		fmt.Println("Error: --sync-folder is required")
		flag.Usage()
		os.Exit(1)
	}

	if *portNumber == "" {
		fmt.Println("Error: --port is required")
		flag.Usage()
		os.Exit(1)
	}

	// Convert chunk size from kilobytes to bytes
	chunkSize := *chunkSizeKB * 1024

	// Output the configurations
	log.Infof("Sync Folder  : %s", *syncFolder)
	log.Infof("Chunk Size   : %d bytes", chunkSize)
	log.Infof("Sync Interval: %v", *syncInterval)
	log.Infof("Port Number  : %s", *portNumber)

	// Initialize the configuration
	conf.AppConfig = conf.Config{
		SyncFolder:   *syncFolder,
		ChunkSize:    chunkSize,
		SyncInterval: *syncInterval,
		Port:         *portNumber,
	}
}

func initDB() *badger.DB {
	opts := badger.DefaultOptions("./badgerdb") // Set your DB path
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatalf("Failed to open BadgerDB: %v", err)
	}
	return db
}

func initServices(db *badger.DB) (*servers.Mdns, *servers.Meta, *servers.FileData, *servers.Conn, *servers.Grpc) {
	// Initialize services without dependencies that cause circular references
	mdns := servers.NewMdns()
	meta := servers.NewMeta(db, mdns)
	file := servers.NewFile(meta, mdns)
	grpc := servers.NewGrpc(conf.AppConfig.SyncFolder, mdns, meta, file, conf.AppConfig.Port)

	// Now initialize conn, passing in required interfaces
	conn := servers.NewConn()

	// Set conn in services that need it via setter methods
	mdns.SetConn(conn)
	meta.SetConn(conn)
	file.SetConn(conn)

	return mdns, meta, file, conn, grpc
}

func startServices(ctx context.Context, wg *sync.WaitGroup, mdns *servers.Mdns, meta *servers.Meta, file *servers.FileData, conn *servers.Conn, grpc *servers.Grpc) {
	// Start Grpc
	grpc.Start()

	// Scan existing files
	meta.Scan()

	// Start Mdns
	wg.Add(1)
	go mdns.Start(ctx, wg)

	// Start Conn
	conn.Start()

	// Start Mdns Ping
	go mdns.Ping(ctx, wg)

	// Start FileData
	wg.Add(1)
	go file.Start(ctx, wg)
}

func waitForShutdownSignal(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for a signal
	sig := <-sigChan
	log.Infof("Received signal: %s. Shutting down...", sig)

	// Cancel the context to stop all running goroutines
	cancel()
}
