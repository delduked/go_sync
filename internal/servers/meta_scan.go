package servers

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
)

// PreScanAndStoreMetaData scans all files in a directory and stores metadata in memory and BadgerDB.
func (m *Meta) PreScanAndStoreMetaData(dir string) error {

	// check if the metadata map is empty before loading metadata from sst backup file
	if len(m.MetaData) != 0 {
		return nil
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Printf("Processing file: %s", path)
			metaData, err := m.getLocalFileMetadata(path, conf.ChunkSize)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
			m.saveMetaData(path, metaData)
		}
		return nil
	})
	return err
}

// PeerMetaData periodically scan for metadata from peers and compares it with local files to identify and handle missing chunks.
func (m *Meta) ScanPeerMetaData(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down list check...")
			return
		case <-ticker.C:
			localFiles, err := pkg.GetFileList()
			if err != nil {
				log.Errorf("Error getting file list: %v", err)
				return
			}

			for _, file := range localFiles {
				if m.PeerData.IsFileInProgress(file) {
					continue
				}
				peerChunks := m.getPeerFileMetaData(file)
				missingChunks := m.missingChunks(file, peerChunks)

				if len(missingChunks) > 0 {
					go func() {
						err := m.ModifyPeerFile(file, missingChunks)
						if err != nil {
							log.Errorf("failed to modify peer file: %v", err)
						}
					}()
				}
			}
		}
	}
}
