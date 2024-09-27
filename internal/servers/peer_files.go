package servers

import (
	"github.com/TypeTerrors/go_sync/pkg"
)

func (pd *PeerData) markFileAsInProgress(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if !pkg.ContainsString(pd.SyncedFiles, fileName) {
		pd.SyncedFiles = append(pd.SyncedFiles, fileName)
	}
}

func (pd *PeerData) markFileAsComplete(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	for i, file := range pd.SyncedFiles {
		if file == fileName {
			pd.SyncedFiles = append(pd.SyncedFiles[:i], pd.SyncedFiles[i+1:]...)
			break
		}
	}
}

func (pd *PeerData) IsFileInProgress(fileName string) bool {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	return pkg.ContainsString(pd.SyncedFiles, fileName)
}
