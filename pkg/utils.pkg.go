package pkg

import "strings"

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
