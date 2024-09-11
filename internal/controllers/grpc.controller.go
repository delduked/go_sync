package controllers

import (
	pb "go_sync/filesync"
	"go_sync/internal/services"
	"io"
)

type FileSyncServer struct {
	pb.UnimplementedFileSyncServiceServer
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
