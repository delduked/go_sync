package test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
)

type FileData struct {
	meta           *Meta
	mdns           *Mdns
	mu             sync.Mutex
	debounceTimers map[string]*time.Timer
	inProgress     map[string]bool
}

func NewFileData(meta *Meta, mdns *Mdns) *FileData {
	return &FileData{
		meta:           meta,
		mdns:           mdns,
		debounceTimers: make(map[string]*time.Timer),
		inProgress:     make(map[string]bool),
	}
}

func (f *FileData) Start() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log.Printf("Watching directory: %s", conf.AppConfig.SyncFolder)
	err = watcher.Add(conf.AppConfig.SyncFolder)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("File created: %s", event.Name)
					go f.HandleFileCreation(event.Name)
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("File modified: %s", event.Name)
					go f.HandleFileModification(event.Name)
				}

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					log.Printf("File deleted: %s", event.Name)
					go f.HandleFileDeletion(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error("Error:", err)
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
	// Check if file is already being processed
	f.mu.Lock()
	if f.inProgress[filePath] {
		f.mu.Unlock()
		return
	}
	f.inProgress[filePath] = true
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		delete(f.inProgress, filePath)
		f.mu.Unlock()
	}()

	// Initialize metadata
	err := f.meta.CreateFileMetaData(filePath)
	if err != nil {
		log.Errorf("Failed to initialize metadata for new file %s: %v", filePath, err)
		return
	}

	// The meta.go's Save channel will handle sending updates to peers
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
	// Check if file is already being processed
	f.mu.Lock()
	if f.inProgress[filePath] {
		f.mu.Unlock()
		return
	}
	f.inProgress[filePath] = true
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		delete(f.inProgress, filePath)
		f.mu.Unlock()
	}()

	// Update metadata
	err := f.meta.CreateFileMetaData(filePath)
	if err != nil {
		log.Errorf("Failed to create/update metadata for file %s: %v", filePath, err)
		return
	}

	// The meta.go's Save channel will handle sending updates to peers
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

	f.DeleteFile(filePath)
}

func (f *FileData) DeleteFile(filePath string) {
	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		f.deleteFileOnPeer(filePath)
		f.meta.DeleteEntireFileMetaData(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
}

func (f *FileData) deleteFileOnPeer(filePath string) error {
	for _, conn := range f.mdns.Clients {
		stream, err := clients.SyncConn(conn)
		if err != nil {
			log.Printf("Failed to initialize stream with peer %s: %v", conn.Target(), err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: filePath,
				},
			},
		})
		if err != nil {
			log.Printf("Failed to send delete request to peer %s: %v", conn.Target(), err)
			continue
		}

		log.Printf("Successfully sent delete request for file %s to peer %s", filePath, conn.Target())
	}

	return nil
}

// deleteFileChunk removes a specific chunk at the provided offset
// It rewrites only the affected part of the file.
func (f *FileData) DeleteFileChunk(filePath string, offset int64) error {
	// Open the file for reading and writing
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for size and other details
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Define the chunk size
	chunkSize := conf.AppConfig.ChunkSize

	// Ensure the offset is valid
	if offset < 0 || offset >= fileInfo.Size() {
		return fmt.Errorf("invalid offset: %d", offset)
	}

	// Calculate how many bytes to move after removing the chunk
	bytesAfterChunk := fileInfo.Size() - (offset + chunkSize)

	if bytesAfterChunk > 0 {
		// Create a buffer to hold the data after the chunk to be deleted
		buffer := make([]byte, bytesAfterChunk)

		// Read the data after the chunk into the buffer
		_, err := file.ReadAt(buffer, offset+chunkSize)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading after chunk: %w", err)
		}

		// Move the data after the chunk to the start of the chunk to overwrite the deleted chunk
		_, err = file.WriteAt(buffer, offset)
		if err != nil {
			return fmt.Errorf("error writing to file after deleting chunk: %w", err)
		}
	}

	// Truncate the file to remove the extra space left at the end
	err = file.Truncate(fileInfo.Size() - chunkSize)
	if err != nil {
		return fmt.Errorf("error truncating file after chunk delete: %w", err)
	}

	// Update the metadata to reflect the chunk removal
	f.meta.DeleteMetaData(filePath, offset)

	log.Printf("Deleted chunk at offset %d from file %s", offset, filePath)
	return nil
}

func (f *FileData) SyncWithPeers() {
	localFileList, err := f.buildLocalFileList()
	if err != nil {
		log.Errorf("Failed to build local file list: %v", err)
		return
	}

	for _, conn := range f.mdns.Clients {
		if conn.Target() == f.mdns.LocalIP {
			continue
		}
		conn.GetState()
		peerFileList, err := f.getPeerfilelist(conn)
		if err != nil {
			log.Errorf("Failed to get file list from %s: %v", conn.Target(), err)
			continue
		}

		missingFiles := f.CompareFileLists(localFileList, peerFileList)
		if len(missingFiles) > 0 {
			log.Infof("Missing files from %s: %v", conn.Target(), missingFiles)
			f.RequestMissingFiles(conn, missingFiles)
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

// sendBytesToPeer sends file data to peers using persistent streams.
func (f *FileData) sendBytesToPeer(fileName string, data []byte, offset int64, isNewFile bool, totalChunks int64) error {
	f.mdns.mu.Lock()
	defer f.mdns.mu.Unlock()

	for _, conn := range f.mdns.Clients {
		stream, err := clients.SyncConn(conn)
		if err != nil {
			log.Printf("Failed to initialize stream with peer %s: %v", conn.Target(), err)
			continue
		}
		// Attempt to send the chunk with retries
		const maxRetries = 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err := stream.Send(&pb.FileSyncRequest{
				Request: &pb.FileSyncRequest_FileChunk{
					FileChunk: &pb.FileChunk{
						FileName:    fileName,
						ChunkData:   data,
						Offset:      offset,
						IsNewFile:   isNewFile,
						TotalChunks: totalChunks,
					},
				},
			})
			if err != nil {
				log.Printf("Attempt %d: Failed to send chunk to peer %s: %v", attempt, conn.Target(), err)
				if attempt < maxRetries {
					log.Printf("Retrying to send chunk to peer %s...", conn.Target())
					time.Sleep(2 * time.Second) // Wait before retrying
					continue
				} else {
					log.Printf("Exceeded max retries for peer %s. Skipping chunk.", conn.Target())
				}
			} else {
				log.Printf("Successfully sent chunk to peer %s for file %s at offset %d", conn.Target(), fileName, offset)
				break
			}
		}
	}

	return nil
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
	totalChunks := (fileSize + conf.AppConfig.ChunkSize - 1) / conf.AppConfig.ChunkSize

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

		go f.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset, isNewFile, totalChunks)
		// if err != nil {
		// 	log.Printf("Error sending data to peer for file %s: %v", filePath, err)
		// 	return
		// }
		offset += int64(n)

		if isNewFile {
			isNewFile = false
		}
	}

	f.markFileAsComplete(filePath)
	log.Printf("File %s transfer complete", filePath)
}

func (f *FileData) markFileAsComplete(fileName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inProgress == nil {
		log.Warnf("SyncedFiles map is nil while trying to mark file as complete. Initializing it now.")
		f.inProgress = make(map[string]bool)
	}

	delete(f.inProgress, fileName)
}

// markFileAsInProgress marks a file as being synchronized.
func (f *FileData) markFileAsInProgress(fileName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inProgress == nil {
		log.Warnf("SyncedFiles map is nil. Initializing it now.")
		f.inProgress = make(map[string]bool)
	}

	f.inProgress[fileName] = true
}
func (f *FileData) IsFileInProgress(fileName string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inProgress == nil {
		log.Warnf("SyncedFiles map is nil while checking if file is in progress. Initializing it now.")
		f.inProgress = make(map[string]bool)
		return false
	}

	_, exists := f.inProgress[fileName]
	return exists
}
