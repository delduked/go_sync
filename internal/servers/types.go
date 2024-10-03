// file_payloads.go

package servers // Replace with your actual package name

// FileChunkPayload represents the payload for a FileChunk message.
type FileChunkPayload struct {
	FileName    string
	ChunkData   []byte
	Offset      int64
	IsNewFile   bool
	TotalChunks int64
	TotalSize   int64
}

// FileDeletePayload represents the payload for a FileDelete message.
type FileDeletePayload struct {
	FileName string
	Offset   int64
}

// FileTruncatePayload represents the payload for a FileTruncate message.
type FileTruncatePayload struct {
	FileName string
	Size     int64
}
type FileTransfer struct {
	FileName string
}