package servers

import (
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/cespare/xxhash"
	"github.com/charmbracelet/log"
)

// FileMonitor represents a monitor for a single file.
type FileMonitor struct {
	filePath   string
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	done       chan struct{}
	isNewFile  bool
}

// FileWatcher monitors files in a directory for changes.
type FileWatcher struct {
	monitoredFiles map[string]*FileMonitor
	debounceTimers map[string]*time.Timer // Map to track debounced file events
	fileSizes      map[string]int64
	fileHashes     map[string]string
	inProgress     map[string]bool
	md             *Meta
	pd             *PeerData
	mu             sync.Mutex
	stopChan       chan struct{}
}

func NewFileWatcher(pd *PeerData, md *Meta) *FileWatcher {
	return &FileWatcher{
		monitoredFiles: make(map[string]*FileMonitor),
		debounceTimers: make(map[string]*time.Timer),
		fileSizes:      make(map[string]int64),
		fileHashes:     make(map[string]string),
		inProgress:     make(map[string]bool),
		md:             md,
		pd:             pd,
	}
}

// HandleFileCreation starts monitoring a newly created file.
func (fw *FileWatcher) HandleFileCreation(filePath string) {
	fw.mu.Lock()
	fw.fileSizes[filePath] = 0
	fw.inProgress[filePath] = true
	fw.mu.Unlock()

	// Initialize pipeReader and pipeWriter for this file
	reader, writer := io.Pipe()
	monitor := &FileMonitor{
		filePath:   filePath,
		pipeReader: reader,
		pipeWriter: writer,
		done:       make(chan struct{}),
		isNewFile:  true, // Assuming it's a new file
	}

	fw.mu.Lock()
	fw.monitoredFiles[filePath] = monitor
	fw.mu.Unlock()

	// Start monitoring the file for new data and capture file writes
	go monitor.captureFileWrites()
	go monitor.processCapturedData(fw)

}
func (fw *FileWatcher) HandleFileDeletion(filePath string) {
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	// Debounce deletion handling
	fw.mu.Lock()
	if fw.debounceTimers == nil {
		fw.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := fw.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	fw.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		fw.processFileDeletion(filePath)
		fw.mu.Lock()
		delete(fw.debounceTimers, filePath)
		fw.mu.Unlock()
	})
	fw.mu.Unlock()
}

// HandleFileDeletion stops monitoring a deleted file.
func (fw *FileWatcher) processFileDeletion(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Stop monitoring the file if it's being monitored
	if monitor, exists := fw.monitoredFiles[filePath]; exists {
		monitor.Stop()
		delete(fw.monitoredFiles, filePath)
	}

	// Remove file size and hash entries
	delete(fw.fileSizes, filePath)
	delete(fw.fileHashes, filePath)
	delete(fw.inProgress, filePath)

	// Notify peers about the file deletion
	for _, conn := range fw.pd.Clients {
		if conn.Target() == fw.pd.LocalIP {
			continue // Skip self
		}

		stream, exists := fw.pd.Streams[conn.Target()]
		if !exists {
			log.Printf("No stream found for peer %s to send delete request", conn.Target())
			continue
		}

		err := stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: filePath,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending delete request to peer %s: %v", conn.Target(), err)
		} else {
			log.Printf("Sent delete request to peer %s for file %s", conn.Target(), filePath)
		}
	}
}

// monitorFile starts monitoring a file for modifications.
func (fw *FileWatcher) monitorFile(filePath string) {
	fw.mu.Lock()
	fw.inProgress[filePath] = true
	fw.mu.Unlock()

	defer func() {
		fw.mu.Lock()
		delete(fw.inProgress, filePath)
		fw.mu.Unlock()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fw.stopChan: // Implement a stop channel if needed
			return
		case <-ticker.C:
			fw.mu.Lock()
			prevSize := fw.fileSizes[filePath]
			fw.mu.Unlock()

			fileInfo, err := os.Stat(filePath)
			if err != nil {
				log.Printf("Failed to stat file %s: %v", filePath, err)
				return
			}
			currSize := fileInfo.Size()

			if currSize > prevSize {
				// New data appended
				newDataSize := currSize - prevSize
				offset := prevSize

				fw.mu.Lock()
				fw.fileSizes[filePath] = currSize
				fw.mu.Unlock()

				// Read and send new data
				fw.readAndSendFileData(filePath, offset, newDataSize)
			}
		}
	}
}

// StopMonitoring stops monitoring all files.
func (fw *FileWatcher) StopMonitoring() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	for _, monitor := range fw.monitoredFiles {
		monitor.Stop()
	}
	fw.monitoredFiles = make(map[string]*FileMonitor)
	fw.fileSizes = make(map[string]int64)
	fw.fileHashes = make(map[string]string)
	fw.inProgress = make(map[string]bool)
}

// captureFileWrites captures writes to the file and writes them to the pipe.
func (fm *FileMonitor) captureFileWrites() {
	// Open the file
	file, err := os.OpenFile(fm.filePath, os.O_RDONLY, 0644)
	if err != nil {
		log.Printf("Failed to open file %s: %v", fm.filePath, err)
		return
	}
	defer file.Close()

	// Read from the file and write to the pipe
	buf := make([]byte, 4096)
	for {
		select {
		case <-fm.done:
			fm.pipeWriter.Close()
			return
		default:
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				log.Printf("Error reading file %s: %v", fm.filePath, err)
				return
			}
			if n == 0 {
				// No more data to read
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Write the data to the pipe
			_, err = fm.pipeWriter.Write(buf[:n])
			if err != nil {
				log.Printf("Error writing to pipe for %s: %v", fm.filePath, err)
				return
			}
		}
	}
}

// processCapturedData reads data from the pipe and processes it.
func (fm *FileMonitor) processCapturedData(fw *FileWatcher) {
	defer fm.pipeReader.Close()
	buf := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for {
		select {
		case <-fm.done:
			// Mark the file as no longer in progress
			fw.pd.markFileAsComplete(fm.filePath)
			return
		default:
			n, err := fm.pipeReader.Read(buf)
			if err != nil {
				if err == io.EOF {
					fw.pd.markFileAsComplete(fm.filePath)
					return
				}
				log.Printf("Error reading from pipe for file %s: %v", fm.filePath, err)
				fw.pd.markFileAsComplete(fm.filePath)
				return
			}

			// Send the captured data to peers with rate-limiting
			err = fw.sendBytesToPeer(filepath.Base(fm.filePath), buf[:n], offset, fm.isNewFile, 0)
			if err != nil {
				log.Printf("Error sending data to peer for file %s: %v", fm.filePath, err)
				fw.pd.markFileAsComplete(fm.filePath)
				return
			}

			offset += int64(n)

			// After sending the first chunk, mark the file as not new
			if fm.isNewFile {
				fm.isNewFile = false
			}
		}
	}
}

// Stop signals the monitor to stop monitoring.
func (fm *FileMonitor) Stop() {
	close(fm.done)
}

// transferFile initiates the real-time transfer of a file to peers.
func (fw *FileWatcher) transferFile(filePath string, isNewFile bool) {
	fw.mu.Lock()
	fw.inProgress[filePath] = true
	fw.mu.Unlock()

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

		err = fw.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset, isNewFile, totalChunks)
		if err != nil {
			log.Printf("Error sending data to peer for file %s: %v", filePath, err)
			return
		}
		offset += int64(n)

		if isNewFile {
			isNewFile = false
		}
	}

	fw.mu.Lock()
	delete(fw.inProgress, filePath)
	fw.mu.Unlock()
	log.Printf("File %s transfer complete", filePath)
}

// sendBytesToPeer sends file data to peers using persistent streams.
func (fw *FileWatcher) sendBytesToPeer(fileName string, data []byte, offset int64, isNewFile bool, totalChunks int64) error {
	fw.pd.mu.Lock()
	defer fw.pd.mu.Unlock()

	for _, conn := range fw.pd.Clients {
		host, _, err := net.SplitHostPort(conn.Target())
		if err != nil {
			log.Errorf("Invalid client target %s: %v", conn.Target(), err)
			continue
		}
		if host == fw.pd.LocalIP {
			continue // Skip self
		}

		stream, exists := fw.pd.Streams[conn.Target()]
		if !exists {
			log.Printf("No persistent stream found for peer %s. Attempting to initialize.", conn.Target())
			newStream, err := clients.SyncStream(conn.Target())
			if err != nil {
				log.Printf("Failed to initialize stream with peer %s: %v", conn.Target(), err)
				continue
			}
			fw.pd.Streams[conn.Target()] = newStream
			stream = newStream
			log.Printf("Initialized new stream with peer %s", conn.Target())
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

// HandleFileModification processes modifications to a file.
func (fw *FileWatcher) HandleFileModification(filePath string) {
	fw.mu.Lock()
	if fw.inProgress[filePath] {
		fw.mu.Unlock()
		return // Ignore modifications caused by sync process
	}
	fw.inProgress[filePath] = true
	fw.mu.Unlock()

	defer func() {
		fw.mu.Lock()
		fw.inProgress[filePath] = false
		fw.mu.Unlock()
	}()
	if pkg.IsTemporaryFile(filePath) {
		return
	}

	// Get current file size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Error stating file %s: %v", filePath, err)
		return
	}
	currSize := fileInfo.Size()

	fw.mu.Lock()
	prevSize := fw.fileSizes[filePath]
	fw.fileSizes[filePath] = currSize
	fw.mu.Unlock()

	if currSize > prevSize {
		// New data appended
		newDataSize := currSize - prevSize
		offset := prevSize

		// Initialize pipe for new data transfer
		reader, writer := io.Pipe()
		fw.monitoredFiles[filePath] = &FileMonitor{
			filePath:   filePath,
			pipeReader: reader,
			pipeWriter: writer,
			done:       make(chan struct{}),
			isNewFile:  false,
		}

		// Start capturing new data and sending to peers
		go fw.readAndSendFileDataWithPipe(filePath, offset, newDataSize, writer)
		go fw.monitoredFiles[filePath].processCapturedData(fw)
	} else if currSize < prevSize {
		// File has shrunk
		go fw.handleFileShrunk(filePath, currSize)
	} else {
		// In-place modification detected
		go fw.handleInPlaceModification(filePath)
	}
}
func (fw *FileWatcher) readAndSendFileDataWithPipe(filePath string, offset int64, length int64, writer *io.PipeWriter) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		writer.CloseWithError(err)
		return
	}
	defer file.Close()

	buf := make([]byte, conf.AppConfig.ChunkSize)
	totalRead := int64(0)

	for totalRead < length {
		bytesToRead := length - totalRead
		if bytesToRead > int64(len(buf)) {
			bytesToRead = int64(len(buf))
		}

		n, err := file.ReadAt(buf[:bytesToRead], offset+totalRead)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s: %v", filePath, err)
			writer.CloseWithError(err)
			return
		}
		if n == 0 {
			break
		}

		_, err = writer.Write(buf[:n])
		if err != nil {
			log.Printf("Error writing data to pipe for file %s: %v", filePath, err)
			writer.CloseWithError(err)
			return
		}

		totalRead += int64(n)
	}
	writer.Close() // Close the pipe when done
}

// handleFileShrunk notifies peers to truncate the file.
func (fw *FileWatcher) handleFileShrunk(filePath string, currSize int64) {
	// Notify peers to truncate the file
	err := fw.sendFileTruncateToPeer(filepath.Base(filePath), currSize)
	if err != nil {
		log.Printf("Error sending truncate command for file %s: %v", filePath, err)
	}
}

func (fw *FileWatcher) handleInPlaceModification(filePath string) {

	if pkg.IsTemporaryFile(filePath) {
		return
	}
	// Lock the file to avoid concurrent changes
	fw.mu.Lock()
	if fw.inProgress[filePath] {
		fw.mu.Unlock()
		return // Skip if the file is already being synced
	}
	fw.inProgress[filePath] = true
	fw.mu.Unlock()

	defer func() {
		fw.mu.Lock()
		delete(fw.inProgress, filePath)
		fw.mu.Unlock()
	}()

	// Detect changed chunks using metadata comparison
	changedOffsets, err := fw.md.DetectChangedChunks(filePath, conf.AppConfig.ChunkSize)
	if err != nil {
		log.Printf("Failed to detect changed chunks for %s: %v", filePath, err)
		return
	}

	if len(changedOffsets) == 0 {
		// No changes detected
		return
	}

	// Sync only the changed chunks
	fw.sendChangedChunks(filePath, changedOffsets)
}

func (fw *FileWatcher) sendChangedChunks(filePath string, offsets []int64) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	buf := make([]byte, conf.AppConfig.ChunkSize)

	for _, offset := range offsets {
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s at offset %d: %v", filePath, offset, err)
			continue
		}
		chunkData := buf[:n]

		// Update the metadata
		fw.md.UpdateFileMetaData(filePath, chunkData, offset, int64(len(chunkData)))

		// Send the modified chunk to peers
		err = fw.sendBytesToPeer(filepath.Base(filePath), chunkData, offset, false, 0)
		if err != nil {
			log.Printf("Error sending modified data to peer for file %s: %v", filePath, err)
			return
		}

	}
}

// readAndSendFileData reads specified data from the file and sends it to peers.
func (fw *FileWatcher) readAndSendFileData(filePath string, offset int64, length int64) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	buf := make([]byte, conf.AppConfig.ChunkSize)
	totalRead := int64(0)

	for totalRead < length {
		bytesToRead := length - totalRead
		if bytesToRead > int64(len(buf)) {
			bytesToRead = int64(len(buf))
		}

		n, err := file.ReadAt(buf[:bytesToRead], offset+totalRead)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s: %v", filePath, err)
			return
		}
		if n == 0 {
			break
		}

		// Send the chunk to peers
		err = fw.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset+totalRead, false, 0)
		if err != nil {
			log.Printf("Error sending data to peer for file %s: %v", filePath, err)
			return
		}

		totalRead += int64(n)
	}
}

// sendFileTruncateToPeer notifies peers to truncate the file to a specific size.
func (fw *FileWatcher) sendFileTruncateToPeer(fileName string, size int64) error {
	for _, conn := range fw.pd.Clients {
		if conn.Target() == fw.pd.LocalIP {
			continue // Skip sending to self
		}

		stream, exists := fw.pd.Streams[conn.Target()]
		if !exists {
			log.Printf("No stream found for peer %s to send truncate command", conn.Target())
			continue
		}

		err := stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileTruncate{
				FileTruncate: &pb.FileTruncate{
					FileName: fileName,
					Size:     size,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending truncate command to peer %s: %v", conn.Target(), err)
		} else {
			log.Printf("Sent truncate command to peer %s for file %s to size %d", conn.Target(), fileName, size)
		}
	}
	return nil
}

// computeFileHash computes the XXHash64 hash of a file.
func computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := xxhash.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
