// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.27.3
// source: filesync/filesync.proto

package filesync

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	FileSyncService_SyncFiles_FullMethodName = "/FileSyncService/SyncFiles"
)

// FileSyncServiceClient is the client API for FileSyncService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FileSyncServiceClient interface {
	SyncFiles(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse], error)
}

type fileSyncServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFileSyncServiceClient(cc grpc.ClientConnInterface) FileSyncServiceClient {
	return &fileSyncServiceClient{cc}
}

func (c *fileSyncServiceClient) SyncFiles(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[0], FileSyncService_SyncFiles_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[FileSyncRequest, FileSyncResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_SyncFilesClient = grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse]

// FileSyncServiceServer is the server API for FileSyncService service.
// All implementations must embed UnimplementedFileSyncServiceServer
// for forward compatibility.
type FileSyncServiceServer interface {
	SyncFiles(grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]) error
	mustEmbedUnimplementedFileSyncServiceServer()
}

// UnimplementedFileSyncServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedFileSyncServiceServer struct{}

func (UnimplementedFileSyncServiceServer) SyncFiles(grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]) error {
	return status.Errorf(codes.Unimplemented, "method SyncFiles not implemented")
}
func (UnimplementedFileSyncServiceServer) mustEmbedUnimplementedFileSyncServiceServer() {}
func (UnimplementedFileSyncServiceServer) testEmbeddedByValue()                         {}

// UnsafeFileSyncServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FileSyncServiceServer will
// result in compilation errors.
type UnsafeFileSyncServiceServer interface {
	mustEmbedUnimplementedFileSyncServiceServer()
}

func RegisterFileSyncServiceServer(s grpc.ServiceRegistrar, srv FileSyncServiceServer) {
	// If the following call pancis, it indicates UnimplementedFileSyncServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&FileSyncService_ServiceDesc, srv)
}

func _FileSyncService_SyncFiles_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).SyncFiles(&grpc.GenericServerStream[FileSyncRequest, FileSyncResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_SyncFilesServer = grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]

// FileSyncService_ServiceDesc is the grpc.ServiceDesc for FileSyncService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FileSyncService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "FileSyncService",
	HandlerType: (*FileSyncServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SyncFiles",
			Handler:       _FileSyncService_SyncFiles_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "filesync/filesync.proto",
}
