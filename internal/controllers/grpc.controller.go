package controllers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

type FileSyncServer struct {
	pb.UnimplementedFileSyncServiceServer
	PeerData      *PeerData
	LocalMetaData *Meta
}

// GRPC Route
// SyncFiles handles the bidirectional streaming of files between peers
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
			s.save(req.GetFileChunk(), stream)
		case *pb.FileSyncRequest_FileDelete:
			s.delete(req.GetFileDelete(), stream)
		case *pb.FileSyncRequest_Poll:
			s.poll(req.GetPoll(), stream)
		case *pb.FileSyncRequest_FileList:
			s.list(req.GetFileList(), stream)
		}
	}
}

// GRPC Route
// State handles the server streaming of the local file system state
func (s *FileSyncServer) State(req *pb.StateReq, stream grpc.ServerStreamingServer[pb.StateRes]) error {
	log.Infof("Received file list")

	localFiles, err := pkg.GetFileList()
	if err != nil {
		log.Errorf("Error getting file list: %v", err)
	}

	err = stream.Send(&pb.StateRes{
		Message: localFiles,
	})
	if err != nil {
		log.Errorf("Error sending state response: %v", err)
		return err
	}
	return nil
}

func (s *FileSyncServer) MetaData(stream pb.FileSyncService_MetaDataServer) error {

	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	log.Infof("Meta data request for file: %s", req.FileName)

	if _, ok := s.LocalMetaData.MetaData[req.FileName]; !ok {
		return nil
	}

	for pos, hash := range s.LocalMetaData.MetaData[req.FileName].Chunks {
		err := stream.Send(&pb.FileMetaData{
			FileName:    req.FileName,
			ChunkNumber: pos,
			ChunkHash:   hash,
			ChunkSize:   s.LocalMetaData.MetaData[req.FileName].ChunkSize,
		})
		if err != nil {
			log.Errorf("Error sending state response: %v", err)
			return err
		}
	}

	return nil
}

// service method
// save saves a file chunk to disk
func (s *FileSyncServer) save(req *pb.FileChunk, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Clean(req.FileName)
	log.Printf("Receiving file chunk: %s, Chunk %d of %d", filePath, req.ChunkNumber, req.TotalChunks)

	// Add the file to SyncedFiles (mark as in progress)
	s.PeerData.markFileAsInProgress(filePath)

	// If this is the last chunk, mark the file as complete and don't write further
	if req.ChunkNumber == req.TotalChunks {
		log.Printf("Final chunk received. File %s transfer complete.", filePath)
		s.PeerData.markFileAsComplete(filePath)

		// Send acknowledgment back to the client
		err := stream.SendMsg(&pb.FileSyncResponse{
			Message: fmt.Sprintf("File %s fully transferred", filePath),
		})
		if err != nil {
			log.Errorf("Error sending final acknowledgment: %v", err)
		}
		return // Don't write the chunk since the transfer is complete
	}

	// Open the file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Error opening file: %v", err)
		s.PeerData.markFileAsComplete(filePath)
		return
	}
	defer file.Close()

	// Write the chunk to the file
	_, err = file.Write(req.ChunkData)
	if err != nil {
		log.Errorf("Error writing file: %v", err)
		s.PeerData.markFileAsComplete(filePath)
		return
	}

	// Send acknowledgment for the current chunk
	err = stream.SendMsg(&pb.FileSyncResponse{
		Message: fmt.Sprintf("Chunk %d/%d saved for file %s", req.ChunkNumber, req.TotalChunks, filePath),
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}

// service method
// delete deletes a file from disk
func (s *FileSyncServer) delete(req *pb.FileDelete, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Clean(req.FileName)
	log.Printf("Deleting file: %s", filePath)

	s.PeerData.markFileAsInProgress(filePath)
	err := os.Remove(filePath)
	if err != nil {
		log.Errorf("Error deleting file: %v", err)
		return
	}
	s.PeerData.markFileAsComplete(filePath)

	// Send an acknowledgment back to the client
	err = stream.Send(&pb.FileSyncResponse{
		Message:     fmt.Sprintf("File %s deleted successfully", req.FileName),
		Filedeleted: filePath,
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}

// service method
// list sends a list of files to the client
func (s *FileSyncServer) list(req *pb.FileList, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	log.Infof("Received file list")

	localFiles, err := pkg.GetFileList()
	if err != nil {
		log.Errorf("Error getting file list: %v", err)
	}

	peerFileMap := make(map[string]struct{})
	for _, file := range req.Files {
		peerFileMap[file] = struct{}{}
	}

	// Create a set of local files
	localFileSet := make(map[string]struct{})
	for _, file := range localFiles {
		localFileSet[file] = struct{}{}
	}

	fileToSend := make([]string, 0)

	// if I have a file the peer doesn't have, make a list of files the peer
	// needs to request
	for file := range peerFileMap {
		if _, ok := localFileSet[file]; !ok {
			fileToSend = append(fileToSend, file)
		}
	}

	// Send list to peer
	err = stream.Send(&pb.FileSyncResponse{
		Filestosend: fileToSend,
	})

	// Respond to the list request with a status message
	if err != nil {
		log.Errorf("Error sending list response: %v", err)
	}

}

// service method
// poll responds to a poll request
func (s *FileSyncServer) poll(req *pb.Poll, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	log.Infof("Received poll request: %s", req.Message)

	// Respond to the poll with a status message
	err := stream.SendMsg(&pb.FileSyncResponse{
		Message: "Poll request received successfully",
	})
	if err != nil {
		log.Errorf("Error sending poll response: %v", err)
	}
}
