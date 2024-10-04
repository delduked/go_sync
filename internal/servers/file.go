package servers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
)

type FileData struct {
	meta           *Meta
	mdns           *Mdns
	mu             sync.RWMutex
	conn           *Conn
	debounceTimers map[string]*time.Timer
	inProgress     map[string]bool
}

func NewFile(meta *Meta, mdns *Mdns, conn *Conn) *FileData {
	return &FileData{
		meta:           meta,
		mdns:           mdns,
		conn:           conn,
		debounceTimers: make(map[string]*time.Timer),
		inProgress:     make(map[string]bool),
	}
}

func (f *FileData) Start(ctx context.Context, wg *sync.WaitGroup) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log.Printf("Watching directory: %s", conf.AppConfig.SyncFolder)
	err = watcher.Add(conf.AppConfig.SyncFolder)
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				switch {
				case event.Op&fsnotify.Create == fsnotify.Create:
					log.Printf("File created: %s", event.Name)
					go f.HandleFileCreation(event.Name)
				case event.Op&fsnotify.Write == fsnotify.Write:
					log.Printf("File modified: %s", event.Name)
					go f.HandleFileModification(event.Name)
				case event.Op&fsnotify.Remove == fsnotify.Remove:
					log.Printf("File deleted: %s", event.Name)
					go f.HandleFileDeletion(event.Name)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error("Watcher error:", err)
			case <-ctx.Done():
				watcher.Close()
				return
			}
		}
	}()

	return watcher, nil
}

func (f *FileData) Scan(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(conf.AppConfig.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.SyncWithPeers()
		}
	}
}
func (f *FileData) HandleFileCreation(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		f.handleDebouncedFileCreation(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
}

func (f *FileData) handleDebouncedFileCreation(filePath string) {
	f.markFileAsInProgress(filePath)
	defer f.markFileAsComplete(filePath)

	// Initialize metadata with isNewFile = true
	err := f.meta.CreateFileMetaData(filePath, true)
	if err != nil {
		log.Errorf("Failed to initialize metadata for new file %s: %v", filePath, err)
		return
	}
}

func (f *FileData) HandleFileModification(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		f.handleDebouncedFileModification(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
}

func (f *FileData) handleDebouncedFileModification(filePath string) {
	f.markFileAsInProgress(filePath)
	defer f.markFileAsComplete(filePath)

	// Update metadata with isNewFile = false
	err := f.meta.CreateFileMetaData(filePath, false)
	if err != nil {
		log.Errorf("Failed to update metadata for %s: %v", filePath, err)
		return
	}
}

func (f *FileData) HandleFileDeletion(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	// For deletions, immediate handling might be preferable, but you can also debounce if necessary
	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		f.handleDebouncedFileDeletion(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
}

func (f *FileData) handleDebouncedFileDeletion(filePath string) {
	f.mu.Lock()
	if f.inProgress[filePath] {
		f.mu.Unlock()
		return
	}
	f.inProgress[filePath] = true
	f.mu.Unlock()

	defer f.markFileAsComplete(filePath)

	// Delete metadata and notify peers
	err1, err2 := f.meta.DeleteEntireFileMetaData(filePath)
	if err1 != nil || err2 != nil {
		log.Errorf("Failed to delete metadata for %s: %v %v", filePath, err1, err2)
		return
	}

	f.conn.SendMessage(&pb.FileSyncRequest{
		Request: &pb.FileSyncRequest_FileDelete{
			FileDelete: &pb.FileDelete{
				FileName: filePath,
			},
		},
	})
}

func (f *FileData) SyncWithPeers() {
	localFileList, err := f.buildLocalFileList()
	if err != nil {
		log.Errorf("Failed to build local file list: %v", err)
		return
	}
	// need to make a channel to receive response.
	// can filter responses by type to determine fruther logic after response mssage is received
	for _, conn := range f.conn.peers {
		if conn.Conn.Target() == f.mdns.LocalIP {
			continue
		}
		peerFileList, err := f.getPeerfilelist(conn.Conn)
		if err != nil {
			log.Errorf("Failed to get file list from %s: %v", conn.Conn.Target(), err)
			continue
		}
		missingFiles := f.CompareFileLists(localFileList, peerFileList)
		if len(missingFiles) > 0 {
			// f.RequestMissingFiles(conn, missingFiles)
			for _, fileName := range missingFiles {
				f.conn.SendMessage(FileTransfer{
					FileName: fileName,
				})
			}
		}
	}
}
func (f *FileData) buildLocalFileList() (*pb.FileList, error) {
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
func (f *FileData) RequestMissingFiles(conn *grpc.ClientConn, missingFiles []string) {
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
func (f *FileData) CompareFileLists(localList, peerList *pb.FileList) []string {
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

func (f *FileData) getPeerfilelist(conn *grpc.ClientConn) (*pb.FileList, error) {
	client := pb.NewFileSyncServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.GetFileList(ctx, &pb.GetFileListRequest{})
	if err != nil {
		return nil, err
	}

	return resp.GetFileList(), nil
}

func (f *FileData) transferFile(filePath string, isNewFile bool) {
	f.markFileAsInProgress(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening file %s for transfer: %v", filePath, err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Error getting file info for %s: %v", filePath, err)
		return
	}
	fileSize := fileInfo.Size()
	// totalChunks := (fileSize + conf.AppConfig.ChunkSize - 1) / conf.AppConfig.ChunkSize

	buf := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for {
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s: %v", filePath, err)
			return
		}
		if n == 0 {
			break
		}

		f.conn.SendMessage(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    filePath,
					ChunkData:   buf[:n],
					Offset:      offset,
					IsNewFile:   isNewFile,
					TotalChunks: f.meta.Files[filePath].TotalChunks(),
					TotalSize:   fileSize,
				} ,
			},
		})

		offset += int64(n)

		if isNewFile {
			isNewFile = false
		}
	}

	f.markFileAsComplete(filePath)
	log.Printf("File %s transfer complete", filePath)
}

// CompareMetadata compares previous and current metadata.
// Returns true if changes are detected along with a description of changes.
func (m *Meta) CompareMetadata(prev, curr *FileMetaData) (bool, []string) {
	if prev == nil || curr == nil {
		return true, []string{"Metadata missing for comparison"}
	}

	changes := []string{}

	// Compare file sizes
	if prev.filesize != curr.filesize {
		changes = append(changes, fmt.Sprintf("Size changed from %d to %d", prev.filesize, curr.filesize))
	}

	// Compare chunk hashes
	for offset, currHash := range curr.hashes {
		prevHash, exists := prev.hashes[offset]
		if !exists {
			changes = append(changes, fmt.Sprintf("New chunk added at offset %d", offset))
			continue
		}
		if currHash.Stronghash != prevHash.Stronghash || currHash.Weakhash != prevHash.Weakhash {
			changes = append(changes, fmt.Sprintf("Chunk modified at offset %d", offset))
		}
	}

	// Check for deleted chunks
	for offset := range prev.hashes {
		if _, exists := curr.hashes[offset]; !exists {
			changes = append(changes, fmt.Sprintf("Chunk deleted at offset %d", offset))
		}
	}

	return len(changes) > 0, changes
}

func (f *FileData) markFileAsComplete(fileName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.inProgress, fileName)
}

func (f *FileData) markFileAsInProgress(fileName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.inProgress[fileName] = true
}
func (f *FileData) IsFileInProgress(fileName string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, exists := f.inProgress[fileName]
	return exists
}
