package controllers

import (
	"context"
	"fmt"
	"sync"

	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
)

func (pd *PeerData) StartMDNSDiscovery(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	localIP, localSubnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	log.Infof("Local IP: %s, Subnet: %s", localIP, localSubnet)

	instance := fmt.Sprintf("filesync-%s", localIP)
	serviceType := "_myapp_filesync._tcp"
	domain := "local."
	txtRecords := []string{"version=1.0", "service_id=go_sync"}

	server, err := zeroconf.Register(instance, serviceType, domain, 50051, txtRecords, nil)
	if err != nil {
		log.Fatalf("Failed to register mDNS service: %v", err)
	}
	defer server.Shutdown()

	// Initialize mDNS resolver
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalf("Failed to initialize mDNS resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			for _, ip := range entry.AddrIPv4 {
				if !pkg.IsInSameSubnet(ip.String(), localSubnet) {
					continue
				}

				if ip.String() == localIP || entry.TTL == 0 {
					continue
				}

				if pkg.ValidateService(entry.Text) {
					log.Infof("Discovered valid service at IP: %s", ip.String())
					err := pd.AddClientConnection(ip.String(), "50051")
					if err != nil {
						log.Errorf("Failed to add client connection for %s: %v", ip.String(), err)
					}
				} else {
					log.Warnf("Service at IP %s did not advertise the correct service, skipping...", ip.String())
				}
			}
		}
	}(entries)

	err = resolver.Browse(ctx, serviceType, domain, entries)
	if err != nil {
		log.Fatalf("Failed to browse mDNS: %v", err)
	}

	<-ctx.Done()
	log.Warn("Shutting down mDNS discovery...")
	close(entries)
}
