package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/TypeTerrors/go_sync/internal/servers"
	"github.com/charmbracelet/log"
	badger "github.com/dgraph-io/badger/v3"
)

func main() {
	var wg sync.WaitGroup

	// Initialize BadgerDB
	opts := badger.DefaultOptions("./badgerdb") // Set your DB path
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	peerData := servers.NewPeerData()
	fw := servers.NewFileWatcher(peerData)
	metaData := servers.NewMeta(peerData, db)

	// Step 1: Pre-scan all files, load into memory, and write to BadgerDB
	if err = metaData.PreScanAndStoreMetaData("./sync_folder"); err != nil {
		log.Fatalf("Failed to perform pre-scan and store metadata: %v", err)
	}

	// Step 2: After metadata is loaded and stored, continue with the rest of the application
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go peerData.ScanMdns(ctx, &wg)

	peerData.InitializeStreams()

	wg.Add(1)
	go metaData.ScanLocalMetaData(&wg, ctx)

	wg.Add(1)

	// Create a new SyncServer with syncDir parameter
	server, err := servers.StateServer(metaData, peerData, "50051", "./sync_folder")
	if err != nil {
		log.Fatalf("Failed to create sync server: %v", err)
	}

	go server.PeriodicMetadataExchange(ctx, &wg)

	// Start the server in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(&wg, ctx, peerData, metaData, fw); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	wg.Add(1)
	go peerData.StartPeriodicSync(ctx, &wg)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for a signal
	sig := <-sigChan
	log.Infof("Received signal: %s. Shutting down...", sig)

	// Cancel the context to stop all running goroutines
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()

	log.Info("Application shut down gracefully.")
}
