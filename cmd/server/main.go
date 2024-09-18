package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/TypeTerrors/go_sync/internal/controllers"
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
	defer db.Close() // Ensure BadgerDB is closed when the application shuts down

	peerData := &controllers.PeerData{
		Clients: make([]string, 0),
	}

	// Create a new Meta instance with BadgerDB
	metaData := controllers.NewMeta(peerData, db)

	// Step 1: Pre-scan all files, load into memory, and write to BadgerDB
	err = metaData.PreScanAndStoreMetaData("./sync_folder")
	if err != nil {
		log.Fatalf("Failed to perform pre-scan and store metadata: %v", err)
	}

	// Step 2: After metadata is loaded and stored, continue with the rest of the application

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())

	// Start the mDNS discovery in a separate goroutine
	wg.Add(1)
	go peerData.StartMDNSDiscovery(ctx, &wg)

	// Start the periodic check in a separate goroutine
	wg.Add(1)
	go peerData.PeriodicCheck(ctx, &wg)

	// Create a new SyncServer
	server, err := controllers.StateServer(peerData, "./sync_folder", "50051")
	if err != nil {
		log.Fatalf("Failed to create sync server: %v", err)
	}

	// Start the server in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(&wg, ctx, peerData, metaData); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

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
