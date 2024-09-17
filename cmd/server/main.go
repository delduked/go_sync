package main

import (
	"context"
	"go_sync/internal/controllers"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/charmbracelet/log"
)

func main() {
	var wg sync.WaitGroup

	peerData := &controllers.PeerData{
		Clients: make([]string, 0),
	}

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

	metaData := controllers.NewMeta(peerData)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(&wg, ctx, peerData, metaData); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	wg.Wait()

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
