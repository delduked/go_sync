package pkg

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc/peer"
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
func GetClientIP(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("unable to get peer info")
	}
	return p.Addr.String(), nil
}

func IsTemporaryFile(fileName string) bool {
	baseName := filepath.Base(fileName)
	// Ignore common temporary file patterns
	if strings.Contains(baseName, ".sb-") ||
		strings.HasPrefix(baseName, "~") ||
		strings.HasPrefix(baseName, ".") ||
		strings.HasSuffix(baseName, "~") ||
		strings.HasSuffix(baseName, ".swp") ||
		strings.HasSuffix(baseName, ".tmp") ||
		strings.HasSuffix(baseName, ".bak") ||
		strings.Contains(baseName, ".DS_Store") {
		log.Debugf("Ignoring temporary file:", fileName)
		return true
	}
	log.Debugf("Not a temporary file:", fileName)
	return false
}

// ReadChunk reads a specific chunk of a file based on the provided filename, offset, and chunkSize.
// It returns the bytes read and any error encountered during the operation.
func ReadChunk(filename string, offset int64, chunkSize int64) ([]byte, error) {
	// Open the file in read-only mode
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	// Seek to the specified offset
	_, err = file.Seek(offset, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to offset %d in file %s: %w", offset, filename, err)
	}

	// Initialize a buffer to hold the chunk data
	buffer := make([]byte, chunkSize)

	// Read the chunk data into the buffer
	bytesRead, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read chunk from file %s at offset %d: %w", filename, offset, err)
	}

	// If EOF is reached, adjust the buffer size to the actual bytes read
	if bytesRead < int(chunkSize) {
		buffer = buffer[:bytesRead]
	}

	return buffer, nil
}
