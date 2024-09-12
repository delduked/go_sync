package services

import (
	pb "go_sync/filesync"
	"go_sync/pkg"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func List(req *pb.FileList, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
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
	for file := range localFileSet {
		if _, ok := peerFileMap[file]; !ok {
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
