package servers

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

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

func (pd *PeerData) CompareFileLists(localList, peerList *pb.FileList) []string {
	localFiles := make(map[string]*pb.FileEntry)
	for _, entry := range localList.Files {
		localFiles[entry.FileName] = entry
	}

	var missingFiles []string
	for _, peerEntry := range peerList.Files {
		_, exists := localFiles[peerEntry.FileName]
		if !exists {
			if !peerEntry.IsDeleted {
				missingFiles = append(missingFiles, peerEntry.FileName)
			}
			continue
		}
		// Handle updates or conflicts if needed
	}

	return missingFiles
}

func (pd *PeerData) RequestMissingFiles(conn *grpc.ClientConn, missingFiles []string) {
	client := pb.NewFileSyncServiceClient(conn)
	for _, fileName := range missingFiles {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := client.GetFile(ctx, &pb.RequestFileTransfer{
			FileName: fileName,
		})
		if err != nil {
			log.Errorf("Failed to request file %s from %s: %v", fileName, conn.Target(), err)
		} else {
			log.Infof("Requested file %s from %s", fileName, conn.Target())
		}
	}
}

func (pd *PeerData) SyncWithPeers() {
	localFileList, err := pd.buildLocalFileList()
	if err != nil {
		log.Errorf("Failed to build local file list: %v", err)
		return
	}

	for _, conn := range pd.Clients {
		if conn.Target() == pd.LocalIP {
			continue
		}
		conn.GetState()
		peerFileList, err := pd.getfilelist(conn)
		if err != nil {
			log.Errorf("Failed to get file list from %s: %v", conn.Target(), err)
			continue
		}

		missingFiles := pd.CompareFileLists(localFileList, peerFileList)
		if len(missingFiles) > 0 {
			log.Infof("Missing files from %s: %v", conn.Target(), missingFiles)
			pd.RequestMissingFiles(conn, missingFiles)
		}
	}
}

func (pd *PeerData) buildLocalFileList() (*pb.FileList, error) {
	// Similar to buildFileList in the server implementation
	// Reuse the code or refactor to a common utility function
	files, err := pkg.GetFileList() // Function to get local file paths
	if err != nil {
		return nil, err
	}

	var fileEntries []*pb.FileEntry
	for _, filePath := range files {
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue // Skip if unable to stat file
		}

		fileEntries = append(fileEntries, &pb.FileEntry{
			FileName:     filepath.Base(filePath),
			FileSize:     fileInfo.Size(),
			LastModified: fileInfo.ModTime().Unix(),
		})
	}

	return &pb.FileList{
		Files: fileEntries,
	}, nil
}

func (pd *PeerData) getfilelist(conn *grpc.ClientConn) (*pb.FileList, error) {
	client := pb.NewFileSyncServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.GetFileList(ctx, &pb.GetFileListRequest{})
	if err != nil {
		return nil, err
	}

	return resp.GetFileList(), nil
}
