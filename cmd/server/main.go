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

	sharedData := &controllers.PeerData{
		Clients: make([]string, 0),
	}

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())

	// Start the mDNS discovery in a separate goroutine
	wg.Add(1)
	go sharedData.StartMDNSDiscovery(ctx, &wg)

	// Start the periodic check in a separate goroutine
	wg.Add(1)
	go sharedData.PeriodicCheck(ctx, &wg)

	// Create a new SyncServer
	server, err := controllers.StateServer(sharedData, "./sync_folder", "50051")
	if err != nil {
		log.Fatalf("Failed to create sync server: %v", err)
	}

	// Start the server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(&wg, ctx, sharedData); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for the server to finish
	wg.Wait()

	// Manually add some connections (optional)
	// err := sharedData.AddClientConnection("127.0.0.1", "50051")
	// if err != nil {
	// 	log.Errorf("Failed to add manual connection: %v", err)
	// }

	// Signal handling: Listen for interrupt signals (e.g., Ctrl+C)
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
