package controllers

import (
	"fmt"
	"io"
	"os"

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

func (s *FileSyncServer) ModifyFiles(stream pb.FileSyncService_ModifyFilesServer) error {

	fileBuffers := make(map[string]*os.File) // A map to track open file handles
	defer func() {
		// Close all file handles when done
		for _, file := range fileBuffers {
			file.Close()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			s.PeerData.markFileAsComplete(req.FileName)
			return nil
		}
		if err != nil {
			return err
		}

		s.PeerData.markFileAsInProgress(req.FileName)
		fileName := req.FileName
		chunkData := req.ChunkData
		chunkNumber := req.ChunkNumber
		chunkSize := req.ChunkSize
		totalChunks := req.TotalChunks

		file, exists := fileBuffers[fileName]
		if !exists {
			var err error
			file, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %v", fileName, err)
			}
			fileBuffers[fileName] = file
		}

		offset := chunkNumber * chunkSize

		_, err = file.WriteAt(chunkData, offset)
		if err != nil {
			return fmt.Errorf("failed to write chunk at offset %d for file %s: %v", offset, fileName, err)
		}

		// Optionally, check if all chunks have been received
		if int(chunkNumber+1) == int(totalChunks) {
			log.Printf("File %s has been fully received (%d chunks)", fileName, totalChunks)
			file.Close()
			delete(fileBuffers, fileName) // Remove the file from the open file map
		}

	}
}
