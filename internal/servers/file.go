package servers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
)

// FileDataInterface defines methods that other services need from FileData
type FileDataInterface interface {
	markFileAsInProgress(fileName string)
	markFileAsComplete(fileName string)
	IsFileInProgress(fileName string) bool
	CompareFileLists(localList, peerList *pb.FileList) []string
	BuildLocalFileList() (*pb.FileList, error)
	SetConn(conn ConnInterface)
	getOpenFile(fileName string) (*os.File, error)
	getOrOpenFile(fileName string) (*os.File, error)
}

type FileData struct {
	meta           MetaInterface
	mu             sync.RWMutex
	conn           ConnInterface
	debounceTimers map[string]*time.Timer
	inProgress     map[string]bool
	openFiles      map[string]*os.File
}

func NewFile(meta MetaInterface, mdns MdnsInterface) *FileData {
	return &FileData{
		meta: meta,
		// conn:           conn,
		debounceTimers: make(map[string]*time.Timer),
		inProgress:     make(map[string]bool),
		openFiles:      make(map[string]*os.File),
	}
}
func (f *FileData) SetConn(conn ConnInterface) {
	f.conn = conn
}

func (f *FileData) Start(ctx context.Context, wg *sync.WaitGroup) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log.Infof("Watching directory: %s", conf.AppConfig.SyncFolder)
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

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					log.Infof("File deleted: %s", event.Name)
					go f.HandleFileDeletion(event.Name)
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					log.Infof("File created: %s", event.Name)
					go f.HandleFileCreation(event.Name)
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Infof("File modified: %s", event.Name)
					go f.HandleFileModification(event.Name)
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
		log.Debugf("Handling debounced file creation: %s", filePath)
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
	log.Debugf("Creating metadata for new file: %s", filePath)
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
	f.markFileAsInProgress(filePath)
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.debounceTimers == nil {
		f.debounceTimers = make(map[string]*time.Timer)
	}

	if timer, exists := f.debounceTimers[filePath]; exists {
		timer.Stop()
	}
	log.Debugf("Handling file modification: %s", filePath)
	f.debounceTimers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
		log.Debugf("Handling debounced file modification: %s", filePath)
		f.handleDebouncedFileModification(filePath)
		f.mu.Lock()
		delete(f.debounceTimers, filePath)
		f.mu.Unlock()
	})
}

func (f *FileData) handleDebouncedFileModification(filePath string) {

	// Update metadata with isNewFile = false
	log.Debugf("Updating metadata for modified file: %s", filePath)
	err := f.meta.CreateFileMetaData(filePath, false)
	if err != nil {
		log.Errorf("Failed to update metadata for %s: %v", filePath, err)
		return
	}
	f.markFileAsComplete(filePath)
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

	// Check if the file still exists
	if _, err := os.Stat(filePath); err == nil {
		// File still exists, so it's not actually deleted
		log.Debugf("File %s still exists, skipping deletion", filePath)
		return
	}

	// File does not exist, proceed to delete metadata and notify peers
	log.Debugf("File %s does not exist, proceeding with deletion", filePath)

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

// i don't like this flow of information,
// instead of sending a request for what files the peer has, waiting for the response
// and running a comparison. I should send the files I have to the peer
// and then the peer can calculate which files i am missing
// and I can receive the files from the peer that I need
func (f *FileData) SyncWithPeers() {
	localFileList, err := f.BuildLocalFileList()
	if err != nil {
		log.Errorf("Failed to build local file list: %v", err)
		return
	}

	f.conn.SendMessage(&pb.FileList{
		Files: localFileList.Files,
	})
}

func (f *FileData) BuildLocalFileList() (*pb.FileList, error) {
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
	log.Debugf("Marking file as complete: %s", fileName)
	delete(f.inProgress, fileName)

	if file, exists := f.openFiles[fileName]; exists {
		// Flush data to disk
		err := file.Sync()
		if err != nil {
			log.Errorf("Failed to sync file %s: %v", fileName, err)
		}
		file.Close()
		log.Debug("Flushed data to disk for file:", fileName)
		delete(f.openFiles, fileName)
	}
}

func (f *FileData) markFileAsInProgress(fileName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log.Debugf("Marking file as in progress: %s", fileName)
	f.inProgress[fileName] = true

	// Open the file for writing if not already open
	if _, exists := f.openFiles[fileName]; !exists {
		file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Infof("Failed to open file %s: %v", fileName, err)
			return
		}
		f.openFiles[fileName] = file
	}
}

func (f *FileData) IsFileInProgress(fileName string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	log.Debugf("Checking if file is in progress:", fileName)
	_, exists := f.inProgress[fileName]
	return exists
}

func (f *FileData) getOpenFile(fileName string) (*os.File, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if file, exists := f.openFiles[fileName]; exists {
		return file, nil
	}
	return nil, fmt.Errorf("file %s is not open", fileName)
}
func (f *FileData) getOrOpenFile(fileName string) (*os.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if file, exists := f.openFiles[fileName]; exists {
		return file, nil
	}
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	f.openFiles[fileName] = file
	return file, nil
}
