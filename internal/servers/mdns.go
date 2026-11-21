package servers

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
)

const serviceType = "_gosync-v2._tcp"

// Discovery advertises the node identity, not its IP. Distinct services on the
// same machine are supported, and the advertised port is always respected.
func Discover(ctx context.Context, wg *sync.WaitGroup, id string, port int, connections *Connections) (func(), error) {
	instance := "gosync-" + id
	server, err := zeroconf.Register(instance, serviceType, "local.", port, []string{"protocol=2", "folder=" + connections.engine.FolderID()}, nil)
	if err != nil {
		return nil, err
	}
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		server.Shutdown()
		return nil, err
	}
	entries := make(chan *zeroconf.ServiceEntry, 32)
	if err = resolver.Browse(ctx, serviceType, "local.", entries); err != nil {
		server.Shutdown()
		return nil, err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}
				if entry.Instance == instance || entry.TTL == 0 || entry.Port <= 0 || entry.Port > 65535 {
					continue
				}
				compatible := false
				folder := ""
				for _, txt := range entry.Text {
					if strings.HasPrefix(txt, "folder=") {
						folder = strings.TrimPrefix(txt, "folder=")
					}
					if txt == "protocol=2" {
						compatible = true
					}
				}
				if !compatible || folder != connections.engine.FolderID() {
					continue
				}
				for _, ip := range entry.AddrIPv4 {
					addr := net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port))
					log.Debugf("Discovered peer %s", addr)
					connections.AddPeer(addr)
				}
			}
		}
	}()
	return server.Shutdown, nil
}
func ParsePort(port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return p, nil
}
