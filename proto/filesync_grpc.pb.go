// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.27.3
// source: proto/filesync.proto

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
	FileSyncService_SyncFile_FullMethodName         = "/FileSyncService/SyncFile"
	FileSyncService_HealthCheck_FullMethodName      = "/FileSyncService/HealthCheck"
	FileSyncService_ExchangeMetadata_FullMethodName = "/FileSyncService/ExchangeMetadata"
	FileSyncService_RequestChunks_FullMethodName    = "/FileSyncService/RequestChunks"
	FileSyncService_GetMissingFiles_FullMethodName  = "/FileSyncService/GetMissingFiles"
)

// FileSyncServiceClient is the client API for FileSyncService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FileSyncServiceClient interface {
	SyncFile(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse], error)
	HealthCheck(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[Ping, Pong], error)
	ExchangeMetadata(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[MetadataRequest, MetadataResponse], error)
	RequestChunks(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ChunkRequest, ChunkResponse], error)
	GetMissingFiles(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileList, FileChunk], error)
}

type fileSyncServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFileSyncServiceClient(cc grpc.ClientConnInterface) FileSyncServiceClient {
	return &fileSyncServiceClient{cc}
}

func (c *fileSyncServiceClient) SyncFile(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[0], FileSyncService_SyncFile_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[FileSyncRequest, FileSyncResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_SyncFileClient = grpc.BidiStreamingClient[FileSyncRequest, FileSyncResponse]

func (c *fileSyncServiceClient) HealthCheck(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[Ping, Pong], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[1], FileSyncService_HealthCheck_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[Ping, Pong]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_HealthCheckClient = grpc.BidiStreamingClient[Ping, Pong]

func (c *fileSyncServiceClient) ExchangeMetadata(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[MetadataRequest, MetadataResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[2], FileSyncService_ExchangeMetadata_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[MetadataRequest, MetadataResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_ExchangeMetadataClient = grpc.BidiStreamingClient[MetadataRequest, MetadataResponse]

func (c *fileSyncServiceClient) RequestChunks(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ChunkRequest, ChunkResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[3], FileSyncService_RequestChunks_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[ChunkRequest, ChunkResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_RequestChunksClient = grpc.BidiStreamingClient[ChunkRequest, ChunkResponse]

func (c *fileSyncServiceClient) GetMissingFiles(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[FileList, FileChunk], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &FileSyncService_ServiceDesc.Streams[4], FileSyncService_GetMissingFiles_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[FileList, FileChunk]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_GetMissingFilesClient = grpc.BidiStreamingClient[FileList, FileChunk]

// FileSyncServiceServer is the server API for FileSyncService service.
// All implementations must embed UnimplementedFileSyncServiceServer
// for forward compatibility.
type FileSyncServiceServer interface {
	SyncFile(grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]) error
	HealthCheck(grpc.BidiStreamingServer[Ping, Pong]) error
	ExchangeMetadata(grpc.BidiStreamingServer[MetadataRequest, MetadataResponse]) error
	RequestChunks(grpc.BidiStreamingServer[ChunkRequest, ChunkResponse]) error
	GetMissingFiles(grpc.BidiStreamingServer[FileList, FileChunk]) error
	mustEmbedUnimplementedFileSyncServiceServer()
}

// UnimplementedFileSyncServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedFileSyncServiceServer struct{}

func (UnimplementedFileSyncServiceServer) SyncFile(grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]) error {
	return status.Errorf(codes.Unimplemented, "method SyncFile not implemented")
}
func (UnimplementedFileSyncServiceServer) HealthCheck(grpc.BidiStreamingServer[Ping, Pong]) error {
	return status.Errorf(codes.Unimplemented, "method HealthCheck not implemented")
}
func (UnimplementedFileSyncServiceServer) ExchangeMetadata(grpc.BidiStreamingServer[MetadataRequest, MetadataResponse]) error {
	return status.Errorf(codes.Unimplemented, "method ExchangeMetadata not implemented")
}
func (UnimplementedFileSyncServiceServer) RequestChunks(grpc.BidiStreamingServer[ChunkRequest, ChunkResponse]) error {
	return status.Errorf(codes.Unimplemented, "method RequestChunks not implemented")
}
func (UnimplementedFileSyncServiceServer) GetMissingFiles(grpc.BidiStreamingServer[FileList, FileChunk]) error {
	return status.Errorf(codes.Unimplemented, "method GetMissingFiles not implemented")
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

func _FileSyncService_SyncFile_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).SyncFile(&grpc.GenericServerStream[FileSyncRequest, FileSyncResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_SyncFileServer = grpc.BidiStreamingServer[FileSyncRequest, FileSyncResponse]

func _FileSyncService_HealthCheck_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).HealthCheck(&grpc.GenericServerStream[Ping, Pong]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_HealthCheckServer = grpc.BidiStreamingServer[Ping, Pong]

func _FileSyncService_ExchangeMetadata_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).ExchangeMetadata(&grpc.GenericServerStream[MetadataRequest, MetadataResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_ExchangeMetadataServer = grpc.BidiStreamingServer[MetadataRequest, MetadataResponse]

func _FileSyncService_RequestChunks_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).RequestChunks(&grpc.GenericServerStream[ChunkRequest, ChunkResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_RequestChunksServer = grpc.BidiStreamingServer[ChunkRequest, ChunkResponse]

func _FileSyncService_GetMissingFiles_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FileSyncServiceServer).GetMissingFiles(&grpc.GenericServerStream[FileList, FileChunk]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type FileSyncService_GetMissingFilesServer = grpc.BidiStreamingServer[FileList, FileChunk]

// FileSyncService_ServiceDesc is the grpc.ServiceDesc for FileSyncService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FileSyncService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "FileSyncService",
	HandlerType: (*FileSyncServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SyncFile",
			Handler:       _FileSyncService_SyncFile_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "HealthCheck",
			Handler:       _FileSyncService_HealthCheck_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "ExchangeMetadata",
			Handler:       _FileSyncService_ExchangeMetadata_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "RequestChunks",
			Handler:       _FileSyncService_RequestChunks_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "GetMissingFiles",
			Handler:       _FileSyncService_GetMissingFiles_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "proto/filesync.proto",
}
