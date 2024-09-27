package servers

import "github.com/charmbracelet/log"

// markFileAsInProgress marks a file as being synchronized.
func (pd *PeerData) markFileAsInProgress(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if pd.SyncedFiles == nil {
		log.Warnf("SyncedFiles map is nil. Initializing it now.")
		pd.SyncedFiles = make(map[string]bool)
	}

	pd.SyncedFiles[fileName] = true
}

func (pd *PeerData) markFileAsComplete(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if pd.SyncedFiles == nil {
		log.Warnf("SyncedFiles map is nil while trying to mark file as complete. Initializing it now.")
		pd.SyncedFiles = make(map[string]bool)
	}

	delete(pd.SyncedFiles, fileName)
}

func (pd *PeerData) IsFileInProgress(fileName string) bool {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if pd.SyncedFiles == nil {
		log.Warnf("SyncedFiles map is nil while checking if file is in progress. Initializing it now.")
		pd.SyncedFiles = make(map[string]bool)
		return false
	}

	_, exists := pd.SyncedFiles[fileName]
	return exists
}
