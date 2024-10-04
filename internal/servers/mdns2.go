// mdns.go

package servers

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
)

// Mdns handles mDNS service discovery and integrates with ConnManager.
type Mdns struct {
	LocalIP     string
	Subnet      string
	SyncedFiles map[string]bool // Set to track files being synchronized
	connManager *Conn
}

// NewMdns initializes a new Mdns service with a reference to ConnManager.
func NewMdns(connManager *Conn) *Mdns {
	localIP, subnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	return &Mdns{
		SyncedFiles: make(map[string]bool),
		LocalIP:     localIP,
		Subnet:      subnet,
		connManager: connManager,
	}
}

// Start begins the mDNS service discovery.
func (m *Mdns) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Infof("Local IP: %s, Subnet: %s", m.LocalIP, m.Subnet)

	instance := fmt.Sprintf("filesync-%s", m.LocalIP)
	serviceType := "_myapp_filesync._tcp"
	domain := "local."
	txtRecords := []string{"version=1.0", "service_id=go_sync"}
	port, err := strconv.Atoi(conf.AppConfig.Port)
	if err != nil {
		log.Fatalf("Failed to parse port: %v", err)
	}
	server, err := zeroconf.Register(instance, serviceType, domain, port, txtRecords, nil)
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
			if entry.Instance == instance {
				log.Infof("Skipping own service instance: %s", entry.Instance)
				continue // Skip own service
			}
			for _, ip := range entry.AddrIPv4 {
				if !pkg.IsInSameSubnet(ip.String(), m.Subnet) {
					continue
				}

				if ip.String() == m.LocalIP {
					continue
				}

				addr := ip.String() + ":" + conf.AppConfig.Port

				if entry.TTL == 0 {
					// Service is no longer available, remove the peer
					log.Infof("Service at IP %s is no longer available, removing peer", ip.String())
					m.connManager.RemovePeer(addr)
					continue
				}

				if pkg.ValidateService(entry.Text) {
					log.Infof("Discovered valid service at IP: %s", ip.String())
					m.connManager.AddPeer(addr)
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
