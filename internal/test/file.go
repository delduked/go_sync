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
	files          map[string]FileMonitor
}
type FileMonitor struct {
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	done       chan struct{}
	isNewFile  bool
}

func (f *FileData) captureFileWrites(filePath string) {
	// Open the file
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	// Read from the file and write to the pipe
	buf := make([]byte, 4096)
	for {
		select {
		case <-f.files[filePath].done:
			f.files[filePath].pipeWriter.Close()
			return
		default:
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				log.Printf("Error reading file %s: %v", filePath, err)
				return
			}
			if n == 0 {
				// No more data to read
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Write the data to the pipe
			_, err = f.files[filePath].pipeWriter.Write(buf[:n])
			if err != nil {
				log.Printf("Error writing to pipe for %s: %v", filePath, err)
				return
			}
		}
	}
}

// processCapturedData reads data from the pipe and processes it.
func (f *FileData) processCapturedData(filePath string) {
	defer f.files[filePath].pipeReader.Close()
	buf := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for {
		select {
		case <-f.files[filePath].done:
			// Mark the file as no longer in progress
			f.markFileAsComplete(filePath)
			return
		default:
			n, err := f.files[filePath].pipeReader.Read(buf)
			if err != nil {
				if err == io.EOF {
					f.markFileAsComplete(filePath)
					return
				}
				log.Printf("Error reading from pipe for file %s: %v", filePath, err)
				f.markFileAsComplete(filePath)
				return
			}

			// Send the captured data to peers with rate-limiting
			go f.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset, f.files[filePath].isNewFile, 0)
			// if err != nil {
			// 	log.Printf("Error sending data to peer for file %s: %v", fm.filePath, err)
			// 	fw.pd.markFileAsComplete(fm.filePath)
			// 	return
			// }

			offset += int64(n)

			// After sending the first chunk, mark the file as not new
			fileMeta := f.files[filePath]
			if fileMeta.isNewFile {
				fileMeta.isNewFile = false
				f.files[filePath] = fileMeta
			}
		}
	}
}

func (f *FileData) listen() (*fsnotify.Watcher, error) {
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
	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {

		// not sure about this part
		// should i send the message only to create a new file and handle the modification through the modification event?
		// or should i read the file and send the chunks inside the creation event?
		f.FileCreation(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
	f.mu.Unlock()
}

// HandleFileCreation starts monitoring a newly created file.
func (f *FileData) FileCreation(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}
	f.mu.Lock()
	// f.fileSizes[filePath] = 0
	f.inProgress[filePath] = true
	f.mu.Unlock()

	// Initialize pipeReader and pipeWriter for this file
	reader, writer := io.Pipe()
	monitor := FileMonitor{
		pipeReader: reader,
		pipeWriter: writer,
		done:       make(chan struct{}),
		isNewFile:  true, // Assuming it's a new file
	}

	f.mu.Lock()
	f.files[filePath] = monitor
	f.mu.Unlock()

	// Start monitoring the file for new data and capture file writes
	go f.captureFileWrites(filePath)
	go f.processCapturedData(filePath)
}

func (f *FileData) HandleFileModification(filePath string) {
	// maybe I can use a debounce method and compare the metadata every 500ms after the last modification
	// if the metadata is the same, then I can send the file to the peers
	// if the metadata is different, then I can send the chunks that are different
	// I can also use the debounce method to send the chunks to the peers
	// I can also use the debounce method to send the file to the peers

}

func (f *FileData) HandleFileDeletion(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	// Debounce deletion handling
	f.mu.Lock()
	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}
	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		f.DeleteFileOnPeer(filePath)
		f.meta.DeleteEntireFileMetaData(filePath)
	})
	f.mu.Unlock()
}
func (f *FileData) DeleteFileOnPeer(filePath string) error {
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
