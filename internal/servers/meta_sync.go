package servers

func (m *Meta) MissingChunks(fileName string, peerChunks map[int64]string) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	localMetaData, exists := m.MetaData[fileName]
	if !exists {
		return nil
	}

	var missingOffsets []int64
	for offset, localHash := range localMetaData.Chunks {
		peerHash, exists := peerChunks[offset]
		if !exists || peerHash != localHash {
			missingOffsets = append(missingOffsets, offset)
		}
	}

	return missingOffsets
}
