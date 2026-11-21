package servers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	pb.UnimplementedFileSyncServiceServer
	Engine    *Engine
	ChunkSize int
}

func (s *Service) ListFiles(ctx context.Context, req *pb.Empty) (*pb.FileList, error) {
	if err := s.Engine.RestoreDeletionFloors(req.KnownPaths); err != nil {
		return nil, err
	}
	return s.Engine.Manifest()
}
func (s *Service) HealthCheck(stream pb.FileSyncService_HealthCheckServer) error {
	for {
		ping, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(&pb.Pong{Message: ping.Message}); err != nil {
			return err
		}
	}
}
func validateEntry(f *pb.FileEntry) error {
	if f == nil {
		return fmt.Errorf("missing entry")
	}
	if err := validName(f.FileName); err != nil {
		return err
	}
	if f.Revision == 0 || f.Revision >= 1<<63 || len(f.Origin) != 32 || f.FileSize < 0 || f.Mode > 0777 {
		return fmt.Errorf("invalid file metadata")
	}
	if _, err := hex.DecodeString(f.Origin); err != nil {
		return err
	}
	for id, n := range f.Version {
		if len(id) != 32 || n == 0 || n >= 1<<63 {
			return fmt.Errorf("invalid version vector")
		}
		if _, err := hex.DecodeString(id); err != nil {
			return fmt.Errorf("invalid version identity")
		}
	}
	if !f.IsDeleted && !f.IsDirectory {
		b, err := hex.DecodeString(f.Hash)
		if err != nil || len(b) != 32 || f.Hash != strings.ToLower(f.Hash) {
			return fmt.Errorf("invalid content hash")
		}
	}
	return nil
}
func (s *Service) ReadFile(req *pb.FileEntry, stream pb.FileSyncService_ReadFileServer) error {
	if err := validateEntry(req); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if excluded, err := s.Engine.excluded(req.FileName, req.IsDirectory); err != nil {
		return err
	} else if excluded {
		return status.Error(codes.PermissionDenied, "excluded path")
	}
	if req.IsDeleted || req.IsDirectory {
		return status.Error(codes.InvalidArgument, "not a regular file")
	}
	if err := s.Engine.checkLocalPath(req.FileName); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	s.Engine.mu.Lock()
	current := s.Engine.files[req.FileName]
	if !sameVersion(req, current) {
		s.Engine.mu.Unlock()
		return status.Error(codes.Aborted, "revision changed")
	}
	file, err := s.Engine.root.Open(req.FileName)
	s.Engine.mu.Unlock()
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return status.Error(codes.InvalidArgument, "not a regular file")
	}
	size := s.ChunkSize
	if size <= 0 || size > 1024*1024 {
		size = 64 * 1024
	}
	buf := make([]byte, size)
	var offset int64
	for offset < req.FileSize {
		n, err := file.Read(buf[:min(int64(len(buf)), req.FileSize-offset)])
		s.Engine.metrics.SourceRead.Add(int64(n))
		if n > 0 {
			if sendErr := stream.Send(&pb.FileChunk{ChunkData: buf[:n], Offset: offset}); sendErr != nil {
				return sendErr
			}
			offset += int64(n)
			s.Engine.metrics.PayloadSent.Add(int64(n))
		}
		if err != nil {
			return status.Errorf(codes.Aborted, "file changed during read: %v", err)
		}
	}
	return nil
}

func (e *Engine) pull(ctx context.Context, client pb.FileSyncServiceClient, entry, expected *pb.FileEntry) (result error) {
	start := time.Now()
	e.metrics.Transfers.Add(1)
	defer func() {
		e.metrics.TransferNanos.Add(time.Since(start).Nanoseconds())
		if result != nil {
			e.metrics.Failures.Add(1)
		}
	}()
	if err := validateEntry(entry); err != nil {
		return err
	}
	if err := e.checkLocalPath(entry.FileName); err != nil {
		return err
	}
	if err := e.checkPortableTarget(entry.FileName); err != nil {
		return err
	}
	unlock := e.lockTransfer(entry.FileName)
	defer unlock()
	if entry.IsDeleted || entry.IsDirectory {
		e.discardResume(entry.FileName)
	}
	if entry.IsDeleted || entry.IsDirectory {
		return e.apply(entry, expected, "")
	}
	temp, name, resumed, err := e.resumeStage(entry.FileName)
	if err != nil {
		return err
	}
	defer func() {
		if keepPartial(result) {
			_ = temp.Sync()
		}
		temp.Close()
		if !keepPartial(result) {
			e.discardResume(entry.FileName)
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = e.receiveDelta(ctx, client, entry, temp, resumed)
	if status.Code(err) == codes.Unimplemented {
		if err = temp.Truncate(0); err != nil {
			return err
		}
		if _, err = temp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		err = e.receiveFull(ctx, client, entry, temp)
	}
	if err != nil {
		return err
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	prefix := sha256.New()
	var n int64
	if expected != nil && !expected.IsDeleted && !expected.IsDirectory {
		count, copyErr := io.CopyN(io.MultiWriter(h, prefix), temp, min(expected.FileSize, entry.FileSize))
		n += count
		if copyErr != nil {
			return copyErr
		}
	}
	count, err := io.Copy(h, temp)
	n += count
	e.metrics.VerifyRead.Add(n)
	if err != nil {
		return err
	}
	if n != entry.FileSize || hex.EncodeToString(h.Sum(nil)) != entry.Hash {
		e.Invalidate(entry.FileName)
		return fmt.Errorf("incomplete or corrupt transfer for %s", entry.FileName)
	}
	if err = temp.Chmod(os.FileMode(entry.Mode)); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return e.apply(entry, expected, name, hex.EncodeToString(prefix.Sum(nil)))
}

func (e *Engine) receiveFull(ctx context.Context, client pb.FileSyncServiceClient, entry *pb.FileEntry, temp *os.File) error {
	stream, err := client.ReadFile(ctx, entry)
	if err != nil {
		return err
	}
	var offset int64
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		e.metrics.PayloadReceived.Add(int64(len(chunk.ChunkData)))
		if chunk.Offset != offset || len(chunk.ChunkData) == 0 || int64(len(chunk.ChunkData)) > entry.FileSize-offset {
			return fmt.Errorf("invalid chunk offset or length")
		}
		written, writeErr := temp.Write(chunk.ChunkData)
		e.metrics.StageWrite.Add(int64(written))
		if writeErr != nil {
			return writeErr
		}
		offset += int64(len(chunk.ChunkData))
	}
	if offset != entry.FileSize {
		return fmt.Errorf("incomplete full transfer")
	}
	return nil
}
