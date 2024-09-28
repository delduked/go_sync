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

func main() {
	// Parse command-line flags and initialize configurations
	parseFlags()

	// Initialize BadgerDB
	db := initDB()
	defer db.Close()

	// Initialize core services
	peerData, metaData, fileWatcher := initServices(db)

	// Pre-scan metadata
	preScanMetadata(metaData)

	// Create context and waitgroup for goroutine management
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Start services
	startServices(ctx, &wg, peerData, metaData, fileWatcher)

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
	portNumber := flag.String("port", "50051", "Synchronization interval (optional)")

	// Parse the flags
	flag.Parse()

	// Check if the required flag is provided
	if *syncFolder == "" {
		fmt.Println("Error: --sync-folder is required")
		flag.Usage()
		os.Exit(1)
	}

	if portNumber == nil {
		fmt.Println("Error: --port is required")
		flag.Usage()
		os.Exit(1)
	}

	// Convert chunk size from kilobytes to bytes
	chunkSize := *chunkSizeKB * 1024

	// Output the configurations
	fmt.Printf("Sync Folder  : %s\n", *syncFolder)
	fmt.Printf("Chunk Size   : %d bytes\n", chunkSize)
	fmt.Printf("Sync Interval: %v\n", *syncInterval)
	fmt.Printf("Port Number  : %v\n", *syncInterval)

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

func initServices(db *badger.DB) (*servers.PeerData, *servers.Meta, *servers.FileWatcher) {
	peerData := servers.NewPeerData()
	metaData := servers.NewMeta(peerData, db)
	fileWatcher := servers.NewFileWatcher(peerData, metaData)
	return peerData, metaData, fileWatcher
}

func preScanMetadata(metaData *servers.Meta) {
	if err := metaData.PreScanAndStoreMetaData(conf.AppConfig.SyncFolder); err != nil {
		log.Fatalf("Failed to perform pre-scan and store metadata: %v", err)
	}
}

func startServices(ctx context.Context, wg *sync.WaitGroup, peerData *servers.PeerData, metaData *servers.Meta, fileWatcher *servers.FileWatcher) {
	// Start mDNS scanning
	wg.Add(1)
	go peerData.ScanMdns(ctx, wg)

	// Initialize streams
	peerData.InitializeStreams()

	// Start scanning local metadata periodically
	wg.Add(1)
	go metaData.ScanLocalMetaData(wg, ctx)

	// Create and start the server
	server, err := servers.StateServer(metaData, peerData, "50051", conf.AppConfig.SyncFolder)
	if err != nil {
		log.Fatalf("Failed to create sync server: %v", err)
	}

	// Start periodic metadata exchange
	wg.Add(1)
	go server.PeriodicMetadataExchange(ctx, wg)

	// Start the server (and watch the sync folder)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(wg, ctx, peerData, metaData, fileWatcher); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Start periodic synchronization
	wg.Add(1)
	go peerData.StartPeriodicSync(ctx, wg)
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
