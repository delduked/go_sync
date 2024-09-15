package controllers

import (
	"fmt"
	pb "go_sync/filesync"
	"go_sync/internal/services"
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
			s.Save(req.GetFileChunk(), stream)
		case *pb.FileSyncRequest_FileDelete:
			s.Delete(req.GetFileDelete(), stream)
		case *pb.FileSyncRequest_Poll:
			services.Poll(req.GetPoll(), stream)
		case *pb.FileSyncRequest_FileList:
			services.List(req.GetFileList(), stream)
		}
	}
}

func (s *FileSyncServer) Save(req *pb.FileChunk, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Clean(req.FileName)
	log.Printf("Saving file chunk: %s", filePath)

	// Add the file to SyncedFiles
	s.SharedData.markFileAsInProgress(filePath)

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

func (s *FileSyncServer) Delete(req *pb.FileDelete, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Join("./sync_folder", req.FileName)
	log.Printf("Deleting file: %s", filePath)

	s.SharedData.markFileAsInProgress(filePath)
	err := os.Remove(filePath)
	if err != nil {
		log.Errorf("Error deleting file: %v", err)
		return
	}
	s.SharedData.markFileAsComplete(filePath)

	// Send an acknowledgment back to the client
	err = stream.Send(&pb.FileSyncResponse{
		Message: fmt.Sprintf("File %s deleted successfully", req.FileName),
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}
