package pkg

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

// validateService checks if the discovered service contains the required TXT records
func ValidateService(txtRecords []string) bool {
	// Iterate over the TXT records to find the expected service identifier
	for _, txt := range txtRecords {
		if strings.Contains(txt, "service_id=go_sync") {
			return true
		}
	}
	return false
}
func IsInSameSubnet(ip, subnet string) bool {
	_, subnetNet, err := net.ParseCIDR(subnet)
	if err != nil {
		log.Errorf("Failed to parse subnet %s: %v", subnet, err)
		return false
	}
	parsedIP := net.ParseIP(ip)
	return subnetNet.Contains(parsedIP)
}

func GetLocalIPAndSubnet() (string, string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", fmt.Errorf("unable to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return "", "", fmt.Errorf("unable to get addresses for interface %s: %w", iface.Name, err)
		}

		for _, addr := range addrs {
			ip, netIPNet := parseIPNet(addr)
			if ip != nil && ip.IsGlobalUnicast() && ip.To4() != nil {
				ones, _ := netIPNet.Mask.Size()                            // Get the ones (mask size)
				subnet := fmt.Sprintf("%s/%d", netIPNet.IP.String(), ones) // Use only the ones value
				return ip.String(), subnet, nil
			}
		}
	}

	return "", "", fmt.Errorf("no valid local IP address found")
}

func parseIPNet(addr net.Addr) (net.IP, *net.IPNet) {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP, v
	case *net.IPAddr:
		return v.IP, &net.IPNet{IP: v.IP, Mask: v.IP.DefaultMask()}
	}
	return nil, nil
}
func GetFileList() ([]string, error) {
	var files []string
	err := filepath.Walk("./sync_folder", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get file list: %v", err)
	}

	return files, nil
}

func Contains[T *grpc.ClientConn | string](slice []T, conn T) bool {
	for _, str := range slice {
		if str == conn {
			return true
		}
	}
	return false
}
