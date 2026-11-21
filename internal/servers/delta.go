package servers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultBlockSize = 64 * 1024
const hashBatchSize = 128

// ReadDelta compares the receiver's basis against a bounded prefix. A newer
// append does not invalidate that prefix; rewrites are caught by the final hash.
// No network operation holds Engine.mu, so a slow peer cannot stall scanning.
func (s *Service) ReadDelta(stream pb.FileSyncService_ReadDeltaServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if err = validateEntry(first.File); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	entry := first.File
	if entry.IsDeleted || entry.IsDirectory || first.BlockSize < 1024 || first.BlockSize > 1024*1024 {
		return status.Error(codes.InvalidArgument, "invalid delta header")
	}
	blockSize := first.BlockSize
	basis := make(map[int64][]byte)
	var last int64 = -1
	add := func(items []*pb.ChunkHash) error {
		if len(items) > hashBatchSize {
			return status.Error(codes.InvalidArgument, "basis batch too large")
		}
		for _, item := range items {
			if item == nil || item.Offset < 0 || item.Offset >= entry.FileSize || item.Offset%blockSize != 0 || item.Offset <= last || len(item.Hash) != sha256.Size {
				return status.Error(codes.InvalidArgument, "invalid basis chunk")
			}
			basis[item.Offset] = item.Hash
			last = item.Offset
		}
		return nil
	}
	if err = add(first.Basis); err != nil {
		return err
	}
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if req.File != nil || req.BlockSize != 0 {
			return status.Error(codes.InvalidArgument, "duplicate delta header")
		}
		if err = add(req.Basis); err != nil {
			return err
		}
	}
	file, err := s.Engine.root.Open(entry.FileName)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < entry.FileSize {
		return status.Error(codes.Aborted, "source shrank or changed type")
	}
	whole := sha256.New()
	buffer := make([]byte, blockSize)
	var sent int64
	for offset := int64(0); offset < entry.FileSize; offset += blockSize {
		if err = stream.Context().Err(); err != nil {
			return err
		}
		n := min(blockSize, entry.FileSize-offset)
		chunk := buffer[:n]
		read, readErr := io.ReadFull(file, chunk)
		s.Engine.metrics.SourceRead.Add(int64(read))
		if readErr != nil {
			return status.Error(codes.Aborted, "source changed during transfer")
		}
		whole.Write(chunk)
		sum := sha256.Sum256(chunk)
		if bytes.Equal(basis[offset], sum[:]) {
			s.Engine.metrics.Reused.Add(n)
			continue
		}
		if err = stream.Send(&pb.FileChunk{Offset: offset, ChunkData: chunk}); err != nil {
			return err
		}
		sent += n
		s.Engine.metrics.PayloadSent.Add(n)
	}
	if hex.EncodeToString(whole.Sum(nil)) != entry.Hash {
		return status.Error(codes.Aborted, "source prefix changed; retry a newer revision")
	}
	log.Debugf("Delta %s: sent_bytes=%d reused_bytes=%d", entry.FileName, sent, entry.FileSize-sent)
	return nil
}

// receiveDelta seeds a new staging file from local bytes, hashes those exact
// bytes, and accepts only ordered aligned replacements from the sender. Neither
// the basis nor a partially received delta is ever written into the live file.
func (e *Engine) receiveDelta(ctx context.Context, client pb.FileSyncServiceClient, entry *pb.FileEntry, temp *os.File, resumed bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.ReadDelta(ctx)
	if err != nil {
		return err
	}
	size := e.BlockSize
	if size == 0 {
		size = defaultBlockSize
	}
	if size < 1024 || size > 1024*1024 {
		return fmt.Errorf("invalid local block size")
	}
	if err = stream.Send(&pb.DeltaRequest{File: entry, BlockSize: int64(size)}); err != nil {
		return deltaSendError(stream, err)
	}
	// Opening a local basis may fail (missing file, permissions, type replacement).
	// In that case send no hashes and request the entire bounded snapshot.
	var basis *os.File
	var openErr error
	if resumed {
		basis = temp
	} else {
		basis, openErr = e.root.Open(entry.FileName)
	}
	if openErr == nil {
		if !resumed {
			defer basis.Close()
		}
		info, statErr := basis.Stat()
		if statErr == nil && info.Mode().IsRegular() {
			limit := min(entry.FileSize, info.Size())
			cloned := !resumed && cloneFile(temp, basis)
			var cached [][sha256.Size]byte
			if cloned {
				e.metrics.ReflinkBytes.Add(info.Size())
				after, statErr := basis.Stat()
				if size == defaultBlockSize && statErr == nil && sameAttributes(info, after) {
					cached = e.cachedBlocks(entry.FileName, info)
				}
			}
			buf := make([]byte, size)
			batch := make([]*pb.ChunkHash, 0, hashBatchSize)
			for offset := int64(0); offset < limit; {
				length := min(int64(size), limit-offset)
				var hash [sha256.Size]byte
				index := int(offset / int64(size))
				canCache := cloned && index < len(cached) && (length == int64(size) || limit == info.Size())
				if canCache {
					hash = cached[index]
				} else {
					reader := basis
					if cloned {
						reader = temp
					}
					n, readErr := reader.ReadAt(buf[:length], offset)
					e.metrics.BasisRead.Add(int64(n))
					if n == 0 {
						break
					}
					length = int64(n)
					if !cloned && !resumed {
						written, err := temp.WriteAt(buf[:n], offset)
						e.metrics.StageWrite.Add(int64(written))
						if err != nil {
							return err
						}
					}
					hash = sha256.Sum256(buf[:n])
					if readErr != nil && readErr != io.EOF {
						return readErr
					}
				}
				batch = append(batch, &pb.ChunkHash{Offset: offset, Hash: hash[:]})
				offset += length
				if len(batch) == hashBatchSize {
					if err = stream.Send(&pb.DeltaRequest{Basis: batch}); err != nil {
						return deltaSendError(stream, err)
					}
					batch = nil
				}
				if length < int64(size) {
					break
				}
			}
			if len(batch) > 0 {
				if err = stream.Send(&pb.DeltaRequest{Basis: batch}); err != nil {
					return deltaSendError(stream, err)
				}
			}
		}
	}
	if err = stream.CloseSend(); err != nil {
		return deltaSendError(stream, err)
	}
	if err = temp.Truncate(entry.FileSize); err != nil {
		return err
	}
	last := int64(-1)
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		e.metrics.PayloadReceived.Add(int64(len(chunk.ChunkData)))
		if chunk.Offset < 0 || chunk.Offset >= entry.FileSize || chunk.Offset%int64(size) != 0 || chunk.Offset <= last || int64(len(chunk.ChunkData)) != min(int64(size), entry.FileSize-chunk.Offset) {
			return fmt.Errorf("invalid delta chunk offset or length")
		}
		written, writeErr := temp.WriteAt(chunk.ChunkData, chunk.Offset)
		e.metrics.StageWrite.Add(int64(written))
		if writeErr != nil {
			return writeErr
		}
		last = chunk.Offset
	}
}

// A server that does not implement ReadDelta may report EOF from Send before
// the real Unimplemented status becomes available from Recv.
func deltaSendError(stream pb.FileSyncService_ReadDeltaClient, err error) error {
	if err == io.EOF {
		_, recvErr := stream.Recv()
		if recvErr != nil {
			return recvErr
		}
	}
	return err
}
