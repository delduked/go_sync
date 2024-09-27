package servers

// markFileAsInProgress marks a file as being synchronized.
func (pd *PeerData) markFileAsInProgress(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	pd.SyncedFiles[fileName] = struct{}{}
}

// markFileAsComplete removes a file from the synchronization tracking.
func (pd *PeerData) markFileAsComplete(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	delete(pd.SyncedFiles, fileName)
}

// IsFileInProgress checks if a file is currently being synchronized.
func (pd *PeerData) IsFileInProgress(fileName string) bool {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	_, exists := pd.SyncedFiles[fileName]
	return exists
}
