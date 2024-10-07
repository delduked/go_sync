package servers

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	"github.com/charmbracelet/log"
)

// GrpcInterface defines methods that other services need from Grpc
type GrpcInterface interface {
	Start()
	Stop()
	HandleFileChunk(chunk *pb.FileChunk) error
}

type Grpc struct {
	pb.UnimplementedFileSyncServiceServer
	grpcServer *grpc.Server
	mdns       MdnsInterface
	meta       MetaInterface
	file       FileDataInterface
	listener   net.Listener
	syncDir    string
	port       string
	wg         *sync.WaitGroup
}

func NewGrpc(syncDir string, mdns *Mdns, meta MetaInterface, file FileDataInterface, port string) *Grpc {

	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		panic(fmt.Sprintf("failed to listen on port %s: %v", port, err))
	}

	return &Grpc{
		grpcServer: grpc.NewServer(),
		syncDir:    syncDir,
		mdns:       mdns,
		meta:       meta,
		file:       file,
		listener:   listener,
		port:       port,
		wg:         &sync.WaitGroup{},
	}
}

func (g *Grpc) Start() {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		log.Infof("Starting gRPC server on port %s...", g.port)
		pb.RegisterFileSyncServiceServer(g.grpcServer, g) // Registering on s.grpcServer
		if err := g.grpcServer.Serve(g.listener); err != nil {
			panic(fmt.Sprintf("failed to serve gRPC server: %v", err))
		}
	}()
}

// Stop gracefully stops the gRPC server
func (g *Grpc) Stop() {
	g.grpcServer.GracefulStop()
	log.Info("gRPC server stopped gracefully.")
}

// Implement the SyncFile method as per the generated interface
func (g *Grpc) SyncFile(stream pb.FileSyncService_SyncFileServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving from stream: %v", err)
			return err
		}

		switch req := req.GetRequest().(type) {
		case *pb.FileSyncRequest_FileChunk:
			g.file.markFileAsInProgress(req.FileChunk.FileName)
			err := g.handleFileChunk(req.FileChunk)
			if err != nil {
				log.Errorf("Error handling file chunk: %v", err)
				return err
			}
		case *pb.FileSyncRequest_FileDelete:
			g.file.markFileAsInProgress(req.FileDelete.FileName)
			err := g.handleFileDelete(req.FileDelete)
			if err != nil {
				log.Errorf("Error handling file delete: %v", err)
				return err
			}
			if req.FileDelete.Offset != 0 {
				stream.Send(&pb.FileSyncResponse{
					Message: fmt.Sprintf("Chunk %s deleted in file %v on peer %v", req.FileDelete.FileName, req.FileDelete.Offset, g.mdns.LocalIp()),
				})
			} else {
				stream.Send(&pb.FileSyncResponse{
					Message: fmt.Sprintf("File %s deleted on peer %v", req.FileDelete.FileName, g.mdns.LocalIp()),
				})
			}
		case *pb.FileSyncRequest_FileTruncate:
			err := g.handleFileTruncate(req.FileTruncate)
			if err != nil {
				log.Errorf("Error handling file truncate: %v", err)
				return err
			}
		default:
			log.Warnf("Received unknown request type")
		}
	}
}

func (g *Grpc) ExchangeMetadata(stream pb.FileSyncService_ExchangeMetadataServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving metadata request: %v", err)
			return err
		}

		fileName := req.FileName

		metaData, err := g.meta.GetEntireFileMetaData(fileName)
		if err != nil {
			log.Errorf("Error getting metadata for file %s: %v", fileName, err)
			err := stream.Send(&pb.MetadataResponse{
				FileName: fileName,
				Chunks:   []*pb.ChunkMetadata{},
			})
			if err != nil {
				log.Errorf("Error sending metadata response: %v", err)
			}
			continue
		} else {
			// Prepare the chunks metadata
			var chunkMetadataList []*pb.ChunkMetadata
			for offset, hash := range metaData {
				chunkMetadataList = append(chunkMetadataList, &pb.ChunkMetadata{
					Offset:       offset,
					Hash:         hash.Stronghash,
					WeakChecksum: hash.Weakhash,
				})
			}

			// Send the metadata response
			err = stream.Send(&pb.MetadataResponse{
				FileName: fileName,
				Chunks:   chunkMetadataList,
			})
		}

		if err != nil {
			log.Errorf("Error sending metadata response: %v", err)
			return err
		} else {
			log.Debugf("Sent metadata response for file", fileName)
		}
	}
}

func (g *Grpc) RequestChunks(stream pb.FileSyncService_RequestChunksServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving chunk request: %v", err)
			return err
		}

		fileName := req.FileName
		offsets := req.Offsets

		filePath := filepath.Join(g.syncDir, fileName)
		file, err := os.Open(filePath)
		if err != nil {
			log.Infof("Error opening file %s: %v", filePath, err)
			continue
		}
		defer file.Close()

		for _, offset := range offsets {
			chunkData := make([]byte, conf.AppConfig.ChunkSize)
			log.Debugf("Reading chunk at offset %d for file %s", offset, fileName)
			n, err := file.ReadAt(chunkData, offset)
			if err != nil && err != io.EOF {
				log.Infof("Error reading file %s at offset %d: %v", filePath, offset, err)
				continue
			}

			err = stream.Send(&pb.ChunkResponse{
				FileName:  fileName,
				Offset:    offset,
				ChunkData: chunkData[:n],
			})
			if err != nil {
				log.Errorf("Error sending chunk response: %v", err)
				return err
			}
		}
	}
}

// func (g *Grpc) GetFileList(ctx context.Context, req *pb.GetFileListRequest) (*pb.GetFileListResponse, error) {
// 	fileList, err := g.buildFileList()
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &pb.GetFileListResponse{
// 		FileList: fileList,
// 	}, nil
// }

// func (g *Grpc) buildFileList() (*pb.FileList, error) {
// 	files, err := pkg.GetFileList() // Function to get local file paths
// 	if err != nil {
// 		return nil, err
// 	}

// 	var fileEntries []*pb.FileEntry
// 	for _, filePath := range files {
// 		fileInfo, err := os.Stat(filePath)
// 		if err != nil {
// 			continue // Skip if unable to stat file
// 		}

// 		fileEntries = append(fileEntries, &pb.FileEntry{
// 			FileName:     filepath.Base(filePath),
// 			FileSize:     fileInfo.Size(),
// 			LastModified: fileInfo.ModTime().Unix(),
// 		})
// 	}

// 	return &pb.FileList{
// 		Files: fileEntries,
// 	}, nil
// }

func (g *Grpc) HealthCheck(stream pb.FileSyncService_HealthCheckServer) error {
	for {
		recv, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving health check request: %v", err)
		}
		log.Debugf("Received ping sent from %s to %s at %v", recv.From, recv.To, recv.Time)
		now := time.Now().Unix()
		ctx, ok := peer.FromContext(stream.Context())
		if !ok {
			log.Errorf("Error getting peer from context: %v", err)
			stream.Send(&pb.Pong{
				Message: fmt.Sprintf("Sent pong from %v at %v", g.mdns.LocalIp(), now),
				From:    g.mdns.LocalIp(),
				To:      "",
				Time:    now,
			})
		} else {
			stream.Send(&pb.Pong{
				Message: fmt.Sprintf("Sent pong from %v to %v at %v", g.mdns.LocalIp(), ctx.Addr.String(), now),
				From:    g.mdns.LocalIp(),
				To:      ctx.Addr.String(),
				Time:    now,
			})
		}
	}
}

// Handler for FileChunk messages
func (g *Grpc) handleFileChunk(chunk *pb.FileChunk) error {
	filePath := chunk.FileName
	g.file.markFileAsInProgress(filePath)

	// Open or get the file
	file, err := g.file.getOrOpenFile(chunk.FileName)
	if err != nil {
		log.Printf("Failed to get or open file %s: %v", filePath, err)
		return err
	}

	// Write the chunk data at the specified offset
	_, err = file.WriteAt(chunk.ChunkData, chunk.Offset)
	if err != nil {
		log.Infof("Failed to write to file %s at offset %d: %v", filePath, chunk.Offset, err)
		return err
	} else {
		log.Debugf("Wrote chunk to file %s at offset %d", filePath, chunk.Offset)
	}

	// Update the metadata
	g.SaveMetaData(filePath, chunk.ChunkData, chunk.Offset)

	// Check if all chunks have been received
	if g.meta.allChunksReceived(chunk.FileName, chunk.TotalChunks) {
		log.Printf("All chunks received for file %s", chunk.FileName)
		g.file.markFileAsComplete(chunk.FileName)
	}

	log.Infof("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)
	return nil
}

// handleFileDelete deletes the specified file.
func (g *Grpc) handleFileDelete(fileDelete *pb.FileDelete) error {
	filePath := filepath.Clean(fileDelete.FileName)
	if fileDelete.Offset != 0 {
		// Delete specific chunk
		err := g.DeleteFileChunk(filePath, fileDelete.Offset)
		if err != nil {
			log.Infof("Error deleting chunk at offset %d in file %s: %v", fileDelete.Offset, filePath, err)
			return err
		}
		log.Infof("Deleted chunk at offset %d in file %s as per request", fileDelete.Offset, filePath)
	} else {
		// Delete entire file
		err := os.Remove(filePath)
		if err != nil {
			log.Infof("Error deleting file %s: %v", filePath, err)
			return err
		}

		g.meta.DeleteEntireFileMetaData(filePath)

		g.file.markFileAsComplete(filePath)

		log.Infof("Deleted file %s as per request", filePath)
	}

	return nil
}

func (g *Grpc) GetMissingFiles(stream pb.FileSyncService_GetMissingFilesServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving missing files request: %v", err)
			return err
		}

		localFileList, err := g.file.BuildLocalFileList()
		if err != nil {
			log.Errorf("Failed to build local file list: %v", err)
			return err
		}

		missingFiles := g.file.CompareFileLists(localFileList, req)
		if len(missingFiles) > 0 {
			for _, fileName := range missingFiles {
				g.transferFile(fileName, stream, true)
			}
		}
	}
}

// handleFileTruncate truncates the specified file to the given size.
func (g *Grpc) handleFileTruncate(fileTruncate *pb.FileTruncate) error {
	filePath := filepath.Join(g.syncDir, filepath.Clean(fileTruncate.FileName))
	err := os.Truncate(filePath, fileTruncate.Size)
	if err != nil {
		log.Infof("Error truncating file %s to size %d: %v", filePath, fileTruncate.Size, err)
		return err
	}
	log.Infof("Truncated file %s to size %d as per request", filePath, fileTruncate.Size)
	return nil
}

func (g *Grpc) DeleteFileChunk(filePath string, offset int64) error {
	// Open the file for reading and writing
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for size and other details
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Define the chunk size
	chunkSize := conf.AppConfig.ChunkSize

	// Ensure the offset is valid
	if offset < 0 || offset >= fileInfo.Size() {
		return fmt.Errorf("invalid offset: %d", offset)
	}

	// Calculate how many bytes to move after removing the chunk
	bytesAfterChunk := fileInfo.Size() - (offset + chunkSize)

	if bytesAfterChunk > 0 {
		// Create a buffer to hold the data after the chunk to be deleted
		buffer := make([]byte, bytesAfterChunk)

		// Read the data after the chunk into the buffer
		_, err := file.ReadAt(buffer, offset+chunkSize)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading after chunk: %w", err)
		}

		// Move the data after the chunk to the start of the chunk to overwrite the deleted chunk
		_, err = file.WriteAt(buffer, offset)
		if err != nil {
			return fmt.Errorf("error writing to file after deleting chunk: %w", err)
		}
	}

	// Truncate the file to remove the extra space left at the end
	err = file.Truncate(fileInfo.Size() - chunkSize)
	if err != nil {
		return fmt.Errorf("error truncating file after chunk delete: %w", err)
	}

	err1, err2 := g.DeleteMetadata(filePath, offset)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("error deleting metadata: %v, %v", err1, err2)
	}

	log.Infof("Deleted chunk at offset %d from file %s", offset, filePath)
	return nil
}

func (g *Grpc) DeleteMetadata(filePath string, offset int64) (error, error) {
	err1 := g.meta.DeleteMetaDataFromMem(filePath, offset)
	err2 := g.meta.DeleteMetaDataFromDB(filePath, offset)
	return err1, err2
}

func (g *Grpc) SaveMetaData(filename string, chunk []byte, offset int64) error {
	// Save new metadata
	g.meta.SaveMetaDataToMem(filename, chunk, offset)
	g.meta.SaveMetaDataToDB(filename, chunk, offset)

	return nil
}

func (g *Grpc) transferFile(filePath string, stream pb.FileSyncService_GetMissingFilesServer, isNewFile bool) {

	file, err := os.Open(filePath)
	if err != nil {
		log.Infof("Error opening file %s for transfer: %v", filePath, err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Infof("Error getting file info for %s: %v", filePath, err)
		return
	}
	fileSize := fileInfo.Size()

	buf := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for {
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			log.Infof("Error reading file %s: %v", filePath, err)
			return
		}
		if n == 0 {
			break
		}

		totalchunks, err := g.meta.TotalChunks(filePath)
		if err != nil {
			log.Infof("Error getting total chunks for file %s: %v", filePath, err)
			return
		}

		stream.Send(&pb.FileChunk{
			FileName:    filePath,
			ChunkData:   buf[:n],
			Offset:      offset,
			IsNewFile:   isNewFile,
			TotalChunks: totalchunks,
			TotalSize:   fileSize,
		})

		offset += int64(n)

		if isNewFile {
			isNewFile = false
		}
	}

	log.Infof("File %s transfer complete", filePath)
}
