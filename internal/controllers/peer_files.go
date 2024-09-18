package controllers

import (
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
)

func (pd *PeerData) markFileAsInProgress(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if !pkg.ContainsString(pd.SyncedFiles, fileName) {
		log.Infof("Marking file %s as in progress", fileName)
		pd.SyncedFiles = append(pd.SyncedFiles, fileName)
	} else {
		log.Infof("File %s is already in progress", fileName)
	}
}

func (pd *PeerData) markFileAsComplete(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	for i, file := range pd.SyncedFiles {
		if file == fileName {
			log.Infof("Marking file %s as complete", fileName)
			pd.SyncedFiles = append(pd.SyncedFiles[:i], pd.SyncedFiles[i+1:]...)
			break
		}
	}
}
func (pd *PeerData) IsFileInProgress(fileName string) bool {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	for _, file := range pd.SyncedFiles {
		if file == fileName {
			log.Infof("File %s is in progress", fileName)
			return true
		}
	}
	log.Infof("File %s is not in progress", fileName)
	return false
}
