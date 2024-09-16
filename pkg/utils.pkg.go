package pkg

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// validateService checks if the discovered service contains the required TXT records
func ValidateService(txtRecords []string) bool {
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
				ones, _ := netIPNet.Mask.Size()                            
				subnet := fmt.Sprintf("%s/%d", netIPNet.IP.String(), ones) 
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

func ContainsString[T string](slice []T, conn T) bool {
	for _, item := range slice {
		if item == conn {
			return true
		}
	}
	return false
}

// SubtractValues subtracts the values at each index of the first array from the second array and returns the difference as an array of strings
//
// Example: SubtractValues([]string{"a", "b", "c"}, []string{"a", "c"}) -> []string{"b"}
func SubtractValues(firstParam []string, secondParam []string) []string {
	first := make(map[string]struct{})
	for _, file := range firstParam {
		first[file] = struct{}{}
	}

	second := make(map[string]struct{})
	for _, file := range secondParam {
		second[file] = struct{}{}
	}

	var result []string
	for file := range second {
		if _, ok := first[file]; !ok {
			result = append(result, file)
		}
	}
	return result
}
