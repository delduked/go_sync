package controllers

import (
	pb "go_sync/filesync"
	"go_sync/internal/services"
	"go_sync/pkg"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

type FileSyncServer struct {
	pb.UnimplementedFileSyncServiceServer
	SharedData *SharedData
}

func (s *FileSyncServer) SyncFiles(stream pb.FileSyncService_SyncFilesServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch req.GetRequest().(type) {
		case *pb.FileSyncRequest_FileChunk:
			services.Save(req.GetFileChunk(), stream)
		case *pb.FileSyncRequest_FileDelete:
			services.Delete(req.GetFileDelete(), stream)
		case *pb.FileSyncRequest_FileRename:
			services.Modify(req.GetFileRename(), stream)
		case *pb.FileSyncRequest_Poll:
			services.Poll(req.GetPoll(), stream)
		case *pb.FileSyncRequest_Ack:
			services.Ack(req.GetAck(), stream)
		case *pb.FileSyncRequest_FileList:
			services.List(req.GetFileList(), stream)
		}
	}
}

func (s *FileSyncServer) Save(req *pb.FileChunk, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Clean(req.FileName)
	log.Printf("Saving file chunk: %s", filePath)

	// Add the file to SyncedFiles
	s.SharedData.mu.Lock()
	if pkg.ContainsString(s.SharedData.SyncedFiles, filePath) {
		log.Printf("File %s is already being synchronized. Ignoring.", filePath)
		s.SharedData.mu.Unlock()
		return
	}
	s.SharedData.SyncedFiles = append(s.SharedData.SyncedFiles, filePath)
	s.SharedData.mu.Unlock()

	// Open the file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Error opening file: %v", err)
		s.SharedData.markFileAsComplete(filePath)
		return
	}
	defer file.Close()

	// Write the chunk to the file
	_, err = file.Write(req.ChunkData)
	if err != nil {
		log.Errorf("Error writing file: %v", err)
		s.SharedData.markFileAsComplete(filePath)
		return
	}

	// Check if the transfer is complete
	if req.ChunkNumber == req.TotalChunks {
		log.Printf("File %s transfer complete", filePath)
		s.SharedData.markFileAsComplete(filePath)
	}

	// Send an acknowledgment back to the client
	err = stream.SendMsg(&pb.FileSyncResponse{
		Message: "File chunk saved successfully",
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}
