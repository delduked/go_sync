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
	fileSizes      map[string]int64
	fileHashes     map[string]string
	inProgress     map[string]bool
	pd             *PeerData
	mu             sync.Mutex
}

func NewFileWatcher(pd *PeerData) *FileWatcher {
	return &FileWatcher{
		monitoredFiles: make(map[string]*FileMonitor),
		fileSizes:      make(map[string]int64),
		fileHashes:     make(map[string]string),
		inProgress:     make(map[string]bool),
		pd:             pd,
	}
}

// HandleFileCreation starts monitoring a newly created file.
func (fw *FileWatcher) HandleFileCreation(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Start monitoring the file for modifications
	fw.monitorFile(filePath, true)

	// Start transferring the file immediately
	go fw.transferFile(filePath, true)
}

// HandleFileDeletion stops monitoring a deleted file.
func (fw *FileWatcher) HandleFileDeletion(filePath string) {
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
	for _, ip := range fw.pd.Clients {
		if ip == fw.pd.LocalIP {
			continue // Skip self
		}

		stream, exists := fw.pd.Streams[ip]
		if !exists {
			log.Printf("No stream found for peer %s to send delete request", ip)
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
			log.Printf("Error sending delete request to peer %s: %v", ip, err)
		} else {
			log.Printf("Sent delete request to peer %s for file %s", ip, filePath)
		}
	}
}

// monitorFile starts monitoring a file for modifications.
func (fw *FileWatcher) monitorFile(filePath string, isNewFile bool) {
	if _, exists := fw.monitoredFiles[filePath]; exists {
		return
	}

	// Create a new FileMonitor
	pipeReader, pipeWriter := io.Pipe()
	monitor := &FileMonitor{
		filePath:   filePath,
		pipeReader: pipeReader,
		pipeWriter: pipeWriter,
		done:       make(chan struct{}),
		isNewFile:  isNewFile,
	}
	fw.monitoredFiles[filePath] = monitor

	// Mark the file as in-progress
	fw.pd.markFileAsInProgress(filePath)

	go monitor.captureFileWrites()
	go monitor.processCapturedData(fw)
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
	buf := make([]byte, 4096)
	var offset int64 = 0

	for {
		select {
		case <-fm.done:
			// Mark the file as no longer in-progress
			fw.pd.markFileAsComplete(fm.filePath)
			return
		default:
			n, err := fm.pipeReader.Read(buf)
			if err != nil {
				if err == io.EOF {
					// Mark the file as no longer in-progress
					fw.pd.markFileAsComplete(fm.filePath)
					return
				}
				log.Printf("Error reading from pipe for file %s: %v", fm.filePath, err)
				fw.pd.markFileAsComplete(fm.filePath)
				return
			}

			// Send the captured data to the peer with rate-limiting
			err = fw.sendBytesToPeer(filepath.Base(fm.filePath), buf[:n], offset, fm.isNewFile, 0)
			if err != nil {
				log.Printf("Error sending data to peer for file %s: %v", fm.filePath, err)
				fw.pd.markFileAsComplete(fm.filePath)
				return
			}

			// Update the file size
			fw.mu.Lock()
			fw.fileSizes[fm.filePath] = offset + int64(n)
			fw.mu.Unlock()

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
	totalChunks := (fileSize + conf.ChunkSize - 1) / conf.ChunkSize

	buf := make([]byte, conf.ChunkSize)
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

	log.Printf("File %s transfer complete", filePath)
}

// sendBytesToPeer sends file data to peers using persistent streams.
func (fw *FileWatcher) sendBytesToPeer(fileName string, data []byte, offset int64, isNewFile bool, totalChunks int64) error {
    fw.pd.mu.Lock()
    defer fw.pd.mu.Unlock()

    for _, target := range fw.pd.Clients {
        host, _, err := net.SplitHostPort(target)
        if err != nil {
            log.Errorf("Invalid client target %s: %v", target, err)
            continue
        }
        if host == fw.pd.LocalIP {
            continue // Skip self
        }

        stream, exists := fw.pd.Streams[target]
        if !exists {
            log.Printf("No persistent stream found for peer %s. Attempting to initialize.", target)
            newStream, err := clients.SyncStream(target)
            if err != nil {
                log.Printf("Failed to initialize stream with peer %s: %v", target, err)
                continue
            }
            fw.pd.Streams[target] = newStream
            stream = newStream
            log.Printf("Initialized new stream with peer %s", target)
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
				log.Printf("Attempt %d: Failed to send chunk to peer %s: %v", attempt, target, err)
				if attempt < maxRetries {
					log.Printf("Retrying to send chunk to peer %s...", target)
					time.Sleep(2 * time.Second) // Wait before retrying
					continue
				} else {
					log.Printf("Exceeded max retries for peer %s. Skipping chunk.", target)
				}
			} else {
				log.Printf("Successfully sent chunk to peer %s for file %s at offset %d", target, fileName, offset)
				break
			}
		}
	}

	return nil
}

// HandleFileModification processes modifications to a file.
func (fw *FileWatcher) HandleFileModification(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Get previous size
	prevSize, exists := fw.fileSizes[filePath]
	if !exists {
		prevSize = 0
	}

	// Get current size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Failed to stat file %s: %v", filePath, err)
		return
	}
	currSize := fileInfo.Size()
	fw.fileSizes[filePath] = currSize

	if currSize < prevSize {
		// File has shrunk
		go fw.handleFileShrunk(filePath, currSize, prevSize)
	} else if currSize == prevSize {
		// Possible in-place modification
		go fw.handleInPlaceModification(filePath)
	} else {
		// File has grown
		// Start monitoring for new data
		fw.monitorFile(filePath, false)
	}
}

// handleFileShrunk notifies peers to truncate the file.
func (fw *FileWatcher) handleFileShrunk(filePath string, currSize, prevSize int64) {
	// Notify peers to truncate the file
	err := fw.sendFileTruncateToPeer(filepath.Base(filePath), currSize)
	if err != nil {
		log.Printf("Error sending truncate command for file %s: %v", filePath, err)
	}
}

// handleInPlaceModification detects in-place modifications and sends updated data.
func (fw *FileWatcher) handleInPlaceModification(filePath string) {
	// Compute current hash
	currHash, err := computeFileHash(filePath)
	if err != nil {
		log.Printf("Failed to compute hash for %s: %v", filePath, err)
		return
	}

	fw.mu.Lock()
	prevHash, hasPrevHash := fw.fileHashes[filePath]
	fw.mu.Unlock()

	if hasPrevHash && currHash == prevHash {
		// No changes detected
		return
	}

	// Update the stored hash
	fw.mu.Lock()
	fw.fileHashes[filePath] = currHash
	fw.mu.Unlock()

	// Read the entire file and send it to peers
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Failed to stat file %s: %v", filePath, err)
		return
	}

	fw.readAndSendFileData(filePath, 0, fileInfo.Size())
}

// readAndSendFileData reads specified data from the file and sends it to peers.
func (fw *FileWatcher) readAndSendFileData(filePath string, offset int64, length int64) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	buf := make([]byte, conf.ChunkSize)
	totalRead := int64(0)
	isNewFile := offset == 0

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

		err = fw.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset+totalRead, isNewFile, 0) // TotalChunks can be recalculated if needed
		if err != nil {
			log.Printf("Error sending data to peer for file %s: %v", filePath, err)
			return
		}

		totalRead += int64(n)
		if isNewFile {
			isNewFile = false
		}
	}
}

// sendFileTruncateToPeer notifies peers to truncate the file to a specific size.
func (fw *FileWatcher) sendFileTruncateToPeer(fileName string, size int64) error {
	for _, ip := range fw.pd.Clients {
		if ip == fw.pd.LocalIP {
			continue // Skip sending to self
		}

		stream, exists := fw.pd.Streams[ip]
		if !exists {
			log.Printf("No stream found for peer %s to send truncate command", ip)
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
			log.Printf("Error sending truncate command to peer %s: %v", ip, err)
		} else {
			log.Printf("Sent truncate command to peer %s for file %s to size %d", ip, fileName, size)
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
