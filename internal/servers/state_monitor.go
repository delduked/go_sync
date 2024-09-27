package servers

import (
	"encoding/hex"
	"io"
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
	filePath            string
	pipeReader          *io.PipeReader
	pipeWriter          *io.PipeWriter
	done                chan struct{}
	isNewFile           bool
	lastSent            time.Time     // Tracks the last time data was sent
	modRate             time.Duration // Minimum time between sending updates
	modificationPending bool          // Tracks if a modification is pending
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
		log.Printf("Deleting file %v on peer %v", filePath, ip)

		stream, err := clients.SyncStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %v: %v", ip, err)
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
			log.Printf("Error sending delete request to peer %v: %v", ip, err)
		}
	}
}

// monitorFile starts monitoring a file for modifications.
func (fw *FileWatcher) monitorFile(filePath string, isNewFile bool) {
	if _, exists := fw.monitoredFiles[filePath]; exists {
		return
	}

	// Create a new FileMonitor with rate-limiting for modifications
	pipeReader, pipeWriter := io.Pipe()
	monitor := &FileMonitor{
		filePath:   filePath,
		pipeReader: pipeReader,
		pipeWriter: pipeWriter,
		done:       make(chan struct{}),
		isNewFile:  isNewFile,
		lastSent:   time.Now(),
		modRate:    1 * time.Second, // Minimum 1 second between updates
	}
	fw.monitoredFiles[filePath] = monitor

	// Mark the file as in-progress
	fw.inProgress[filePath] = true

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
			fw.mu.Lock()
			delete(fw.inProgress, fm.filePath)
			fw.mu.Unlock()
			return
		default:
			n, err := fm.pipeReader.Read(buf)
			if err != nil {
				if err == io.EOF {
					// Mark the file as no longer in-progress
					fw.mu.Lock()
					delete(fw.inProgress, fm.filePath)
					fw.mu.Unlock()
					return
				}
				log.Printf("Error reading from pipe for file %s: %v", fm.filePath, err)
				return
			}

			// Rate-limit file modifications
			now := time.Now()
			if now.Sub(fm.lastSent) >= fm.modRate {
				fm.modificationPending = false
				fm.lastSent = now

				// Send the captured data to the peer
				err = fw.sendBytesToPeer(filepath.Base(fm.filePath), buf[:n], offset, fm.isNewFile)
				if err != nil {
					log.Printf("Error sending data to peer for file %s: %v", fm.filePath, err)
					return
				}
				offset += int64(n)

				// After sending the first chunk, mark the file as not new
				if fm.isNewFile {
					fm.isNewFile = false
				}
			} else {
				// Mark the modification as pending
				fm.modificationPending = true
			}
		}
	}
}

// Stop signals the monitor to stop monitoring.
func (fm *FileMonitor) Stop() {
	close(fm.done)
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

// sendBytesToPeer sends file data to the peer.
func (fw *FileWatcher) sendBytesToPeer(fileName string, data []byte, offset int64, isNewFile bool) error {
	// Implement your gRPC client logic here
	// Send fileName, data, offset, and isNewFile flag to the peer

	for _, ip := range fw.pd.Clients {
		stream, err := clients.SyncStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", ip, err)
			continue
		}

		// No need to lock here as we are only reading the size
		size := fw.fileSizes[fileName]

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    fileName,
					ChunkData:   data,
					Offset:      offset,
					IsNewFile:   isNewFile,
					TotalChunks: int64(size+int64(len(data))-1) / int64(conf.ChunkSize),
				},
			},
		})
		if err != nil {
			log.Printf("Error sending chunk to peer %s: %v", ip, err)
		}
	}

	return nil
}

// handleFileShrunk handles the scenario where a file has shrunk.
func (fw *FileWatcher) handleFileShrunk(filePath string, currSize, prevSize int64) {
	// Notify peer to truncate or delete the data beyond currSize
	err := fw.sendFileTruncateToPeer(filepath.Base(filePath), currSize)
	if err != nil {
		log.Printf("Error sending truncate command to peer for file %s: %v", filePath, err)
	}
}

// handleInPlaceModification handles modifications where the file size hasn't changed.
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


// readAndSendFileData reads the specified range of data from the file and sends it to the peer.
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

		err = fw.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset+totalRead, isNewFile)
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


// sendFileTruncateToPeer notifies the peer to truncate the file to a specific size.
func (fw *FileWatcher) sendFileTruncateToPeer(fileName string, size int64) error {
	for _, ip := range fw.pd.Clients {
		stream, err := clients.SyncStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", ip, err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileTruncate{
				FileTruncate: &pb.FileTruncate{
					FileName: fileName,
					Size:     size,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending truncate command to peer %s: %v", ip, err)
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

// Implement real-time file transfer
func (fw *FileWatcher) transferFile(filePath string, isNewFile bool) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening file %s for transfer: %v", filePath, err)
		return
	}
	defer file.Close()

	buf := make([]byte, conf.ChunkSize)
	var offset int64 = 0

	for {
		// Read file in chunks
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s: %v", filePath, err)
			return
		}
		if n == 0 {
			break
		}

		// Send chunk to peer
		err = fw.sendBytesToPeer(filepath.Base(filePath), buf[:n], offset, isNewFile)
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
