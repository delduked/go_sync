// mdns.go

package servers

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
)

type MdnsInterface interface {
	LocalIp() string
	SetConn(conn ConnInterface)
}

// Mdns handles mDNS service discovery and integrates with ConnManager.
type Mdns struct {
	LocalIP     string
	Subnet      string
	SyncedFiles map[string]bool // Set to track files being synchronized
	conn        ConnInterface
	wg          *sync.WaitGroup
}

// NewMdns initializes a new Mdns service with a reference to ConnManager.
func NewMdns() *Mdns {
	localIP, subnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	return &Mdns{
		SyncedFiles: make(map[string]bool),
		LocalIP:     localIP,
		Subnet:      subnet,
		// conn:        conn,
	}
}
func (m *Mdns) SetConn(conn ConnInterface) {
	m.conn = conn
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
					m.conn.RemovePeer(addr)
					continue
				}

				if pkg.ValidateService(entry.Text) {
					log.Infof("Discovered valid service at IP: %s", ip.String())
					m.conn.AddPeer(addr)
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

func (m *Mdns) Ping(ctx context.Context, wg *sync.WaitGroup) {

	m.wg.Add(1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer func() {
			m.wg.Done()
			ticker.Stop()
		}()

		for {
			select {
			case <-ctx.Done():
				log.Warn("Shutting down periodic metadata exchange...")
				return
			case <-ticker.C:

				m.conn.SendMessage(&pb.Ping{
					Message: fmt.Sprintf("Ping from %v at %v", m.LocalIP, time.Now().Unix()),
				})

			}
		}
	}()
}

func (m *Mdns) LocalIp() string {
	return m.LocalIP
}
