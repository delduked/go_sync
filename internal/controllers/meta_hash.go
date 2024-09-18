package controllers

import (
	"fmt"
	"github.com/zeebo/xxh3"
)

// hashPosition calculates the XXH3 hash for a file chunk and returns its hash and position.
func (m *Meta) hashPosition(chunk []byte, offset int64) (string, int64) {
	hash := m.hashChunk(chunk)
	pos := m.chunkPosition(chunk, offset)
	return hash, pos
}

// hashChunk calculates the XXH3 128-bit hash of a given chunk.
func (m *Meta) hashChunk(chunk []byte) string {
	// Calculate the 128-bit XXH3 hash for the chunk
	hash := xxh3.Hash128(chunk)
	// Format the 128-bit hash into a hexadecimal string
	return fmt.Sprintf("%016x%016x", hash.Lo, hash.Hi)
}

// chunkPosition calculates the position of a chunk based on its offset in the file.
func (m *Meta) chunkPosition(chunk []byte, offset int64) int64 {
	return offset / int64(len(chunk))
}
