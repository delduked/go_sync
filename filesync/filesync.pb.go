// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.3
// source: filesync/filesync.proto

package filesync

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type FileSyncRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Request:
	//
	//	*FileSyncRequest_FileChunk
	//	*FileSyncRequest_FileDelete
	//	*FileSyncRequest_FileRename
	//	*FileSyncRequest_Ack
	//	*FileSyncRequest_Poll
	Request isFileSyncRequest_Request `protobuf_oneof:"request"`
}

func (x *FileSyncRequest) Reset() {
	*x = FileSyncRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileSyncRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileSyncRequest) ProtoMessage() {}

func (x *FileSyncRequest) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileSyncRequest.ProtoReflect.Descriptor instead.
func (*FileSyncRequest) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{0}
}

func (m *FileSyncRequest) GetRequest() isFileSyncRequest_Request {
	if m != nil {
		return m.Request
	}
	return nil
}

func (x *FileSyncRequest) GetFileChunk() *FileChunk {
	if x, ok := x.GetRequest().(*FileSyncRequest_FileChunk); ok {
		return x.FileChunk
	}
	return nil
}

func (x *FileSyncRequest) GetFileDelete() *FileDelete {
	if x, ok := x.GetRequest().(*FileSyncRequest_FileDelete); ok {
		return x.FileDelete
	}
	return nil
}

func (x *FileSyncRequest) GetFileRename() *FileRename {
	if x, ok := x.GetRequest().(*FileSyncRequest_FileRename); ok {
		return x.FileRename
	}
	return nil
}

func (x *FileSyncRequest) GetAck() *Acknowledgment {
	if x, ok := x.GetRequest().(*FileSyncRequest_Ack); ok {
		return x.Ack
	}
	return nil
}

func (x *FileSyncRequest) GetPoll() *Poll {
	if x, ok := x.GetRequest().(*FileSyncRequest_Poll); ok {
		return x.Poll
	}
	return nil
}

type isFileSyncRequest_Request interface {
	isFileSyncRequest_Request()
}

type FileSyncRequest_FileChunk struct {
	FileChunk *FileChunk `protobuf:"bytes,1,opt,name=file_chunk,json=fileChunk,proto3,oneof"`
}

type FileSyncRequest_FileDelete struct {
	FileDelete *FileDelete `protobuf:"bytes,2,opt,name=file_delete,json=fileDelete,proto3,oneof"`
}

type FileSyncRequest_FileRename struct {
	FileRename *FileRename `protobuf:"bytes,3,opt,name=file_rename,json=fileRename,proto3,oneof"`
}

type FileSyncRequest_Ack struct {
	Ack *Acknowledgment `protobuf:"bytes,4,opt,name=ack,proto3,oneof"`
}

type FileSyncRequest_Poll struct {
	Poll *Poll `protobuf:"bytes,5,opt,name=poll,proto3,oneof"`
}

func (*FileSyncRequest_FileChunk) isFileSyncRequest_Request() {}

func (*FileSyncRequest_FileDelete) isFileSyncRequest_Request() {}

func (*FileSyncRequest_FileRename) isFileSyncRequest_Request() {}

func (*FileSyncRequest_Ack) isFileSyncRequest_Request() {}

func (*FileSyncRequest_Poll) isFileSyncRequest_Request() {}

type FileChunk struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName  string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
	ChunkData []byte `protobuf:"bytes,2,opt,name=chunk_data,json=chunkData,proto3" json:"chunk_data,omitempty"`
}

func (x *FileChunk) Reset() {
	*x = FileChunk{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileChunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileChunk) ProtoMessage() {}

func (x *FileChunk) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileChunk.ProtoReflect.Descriptor instead.
func (*FileChunk) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{1}
}

func (x *FileChunk) GetFileName() string {
	if x != nil {
		return x.FileName
	}
	return ""
}

func (x *FileChunk) GetChunkData() []byte {
	if x != nil {
		return x.ChunkData
	}
	return nil
}

type FileDelete struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
}

func (x *FileDelete) Reset() {
	*x = FileDelete{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileDelete) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileDelete) ProtoMessage() {}

func (x *FileDelete) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileDelete.ProtoReflect.Descriptor instead.
func (*FileDelete) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{2}
}

func (x *FileDelete) GetFileName() string {
	if x != nil {
		return x.FileName
	}
	return ""
}

type FileRename struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	OldName string `protobuf:"bytes,1,opt,name=old_name,json=oldName,proto3" json:"old_name,omitempty"`
	NewName string `protobuf:"bytes,2,opt,name=new_name,json=newName,proto3" json:"new_name,omitempty"`
}

func (x *FileRename) Reset() {
	*x = FileRename{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileRename) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileRename) ProtoMessage() {}

func (x *FileRename) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileRename.ProtoReflect.Descriptor instead.
func (*FileRename) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{3}
}

func (x *FileRename) GetOldName() string {
	if x != nil {
		return x.OldName
	}
	return ""
}

func (x *FileRename) GetNewName() string {
	if x != nil {
		return x.NewName
	}
	return ""
}

type FileSyncResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *FileSyncResponse) Reset() {
	*x = FileSyncResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileSyncResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileSyncResponse) ProtoMessage() {}

func (x *FileSyncResponse) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileSyncResponse.ProtoReflect.Descriptor instead.
func (*FileSyncResponse) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{4}
}

func (x *FileSyncResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type Acknowledgment struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *Acknowledgment) Reset() {
	*x = Acknowledgment{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Acknowledgment) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Acknowledgment) ProtoMessage() {}

func (x *Acknowledgment) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Acknowledgment.ProtoReflect.Descriptor instead.
func (*Acknowledgment) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{5}
}

func (x *Acknowledgment) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type Poll struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *Poll) Reset() {
	*x = Poll{}
	if protoimpl.UnsafeEnabled {
		mi := &file_filesync_filesync_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Poll) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Poll) ProtoMessage() {}

func (x *Poll) ProtoReflect() protoreflect.Message {
	mi := &file_filesync_filesync_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Poll.ProtoReflect.Descriptor instead.
func (*Poll) Descriptor() ([]byte, []int) {
	return file_filesync_filesync_proto_rawDescGZIP(), []int{6}
}

func (x *Poll) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

var File_filesync_filesync_proto protoreflect.FileDescriptor

var file_filesync_filesync_proto_rawDesc = []byte{
	0x0a, 0x17, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x79, 0x6e, 0x63, 0x2f, 0x66, 0x69, 0x6c, 0x65, 0x73,
	0x79, 0x6e, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xeb, 0x01, 0x0a, 0x0f, 0x46, 0x69,
	0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2b, 0x0a,
	0x0a, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0a, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x48, 0x00, 0x52,
	0x09, 0x66, 0x69, 0x6c, 0x65, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x12, 0x2e, 0x0a, 0x0b, 0x66, 0x69,
	0x6c, 0x65, 0x5f, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x0b, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x48, 0x00, 0x52, 0x0a,
	0x66, 0x69, 0x6c, 0x65, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x12, 0x2e, 0x0a, 0x0b, 0x66, 0x69,
	0x6c, 0x65, 0x5f, 0x72, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x0b, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x48, 0x00, 0x52, 0x0a,
	0x66, 0x69, 0x6c, 0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x23, 0x0a, 0x03, 0x61, 0x63,
	0x6b, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x41, 0x63, 0x6b, 0x6e, 0x6f, 0x77,
	0x6c, 0x65, 0x64, 0x67, 0x6d, 0x65, 0x6e, 0x74, 0x48, 0x00, 0x52, 0x03, 0x61, 0x63, 0x6b, 0x12,
	0x1b, 0x0a, 0x04, 0x70, 0x6f, 0x6c, 0x6c, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x05, 0x2e,
	0x50, 0x6f, 0x6c, 0x6c, 0x48, 0x00, 0x52, 0x04, 0x70, 0x6f, 0x6c, 0x6c, 0x42, 0x09, 0x0a, 0x07,
	0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x47, 0x0a, 0x09, 0x46, 0x69, 0x6c, 0x65, 0x43,
	0x68, 0x75, 0x6e, 0x6b, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4e, 0x61, 0x6d,
	0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x44, 0x61, 0x74, 0x61,
	0x22, 0x29, 0x0a, 0x0a, 0x46, 0x69, 0x6c, 0x65, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x12, 0x1b,
	0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x22, 0x42, 0x0a, 0x0a, 0x46,
	0x69, 0x6c, 0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x6f, 0x6c, 0x64,
	0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6f, 0x6c, 0x64,
	0x4e, 0x61, 0x6d, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x6e, 0x65, 0x77, 0x5f, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6e, 0x65, 0x77, 0x4e, 0x61, 0x6d, 0x65, 0x22,
	0x2c, 0x0a, 0x10, 0x46, 0x69, 0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x2a, 0x0a,
	0x0e, 0x41, 0x63, 0x6b, 0x6e, 0x6f, 0x77, 0x6c, 0x65, 0x64, 0x67, 0x6d, 0x65, 0x6e, 0x74, 0x12,
	0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x20, 0x0a, 0x04, 0x50, 0x6f, 0x6c,
	0x6c, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x32, 0x47, 0x0a, 0x0f, 0x46,
	0x69, 0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x34,
	0x0a, 0x09, 0x53, 0x79, 0x6e, 0x63, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x12, 0x10, 0x2e, 0x46, 0x69,
	0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x11, 0x2e,
	0x46, 0x69, 0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x28, 0x01, 0x30, 0x01, 0x42, 0x14, 0x5a, 0x12, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x79, 0x6e, 0x63,
	0x2f, 0x3b, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x79, 0x6e, 0x63, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_filesync_filesync_proto_rawDescOnce sync.Once
	file_filesync_filesync_proto_rawDescData = file_filesync_filesync_proto_rawDesc
)

func file_filesync_filesync_proto_rawDescGZIP() []byte {
	file_filesync_filesync_proto_rawDescOnce.Do(func() {
		file_filesync_filesync_proto_rawDescData = protoimpl.X.CompressGZIP(file_filesync_filesync_proto_rawDescData)
	})
	return file_filesync_filesync_proto_rawDescData
}

var file_filesync_filesync_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_filesync_filesync_proto_goTypes = []any{
	(*FileSyncRequest)(nil),  // 0: FileSyncRequest
	(*FileChunk)(nil),        // 1: FileChunk
	(*FileDelete)(nil),       // 2: FileDelete
	(*FileRename)(nil),       // 3: FileRename
	(*FileSyncResponse)(nil), // 4: FileSyncResponse
	(*Acknowledgment)(nil),   // 5: Acknowledgment
	(*Poll)(nil),             // 6: Poll
}
var file_filesync_filesync_proto_depIdxs = []int32{
	1, // 0: FileSyncRequest.file_chunk:type_name -> FileChunk
	2, // 1: FileSyncRequest.file_delete:type_name -> FileDelete
	3, // 2: FileSyncRequest.file_rename:type_name -> FileRename
	5, // 3: FileSyncRequest.ack:type_name -> Acknowledgment
	6, // 4: FileSyncRequest.poll:type_name -> Poll
	0, // 5: FileSyncService.SyncFiles:input_type -> FileSyncRequest
	4, // 6: FileSyncService.SyncFiles:output_type -> FileSyncResponse
	6, // [6:7] is the sub-list for method output_type
	5, // [5:6] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_filesync_filesync_proto_init() }
func file_filesync_filesync_proto_init() {
	if File_filesync_filesync_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_filesync_filesync_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*FileSyncRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*FileChunk); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*FileDelete); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*FileRename); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*FileSyncResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[5].Exporter = func(v any, i int) any {
			switch v := v.(*Acknowledgment); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_filesync_filesync_proto_msgTypes[6].Exporter = func(v any, i int) any {
			switch v := v.(*Poll); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_filesync_filesync_proto_msgTypes[0].OneofWrappers = []any{
		(*FileSyncRequest_FileChunk)(nil),
		(*FileSyncRequest_FileDelete)(nil),
		(*FileSyncRequest_FileRename)(nil),
		(*FileSyncRequest_Ack)(nil),
		(*FileSyncRequest_Poll)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_filesync_filesync_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_filesync_filesync_proto_goTypes,
		DependencyIndexes: file_filesync_filesync_proto_depIdxs,
		MessageInfos:      file_filesync_filesync_proto_msgTypes,
	}.Build()
	File_filesync_filesync_proto = out.File
	file_filesync_filesync_proto_rawDesc = nil
	file_filesync_filesync_proto_goTypes = nil
	file_filesync_filesync_proto_depIdxs = nil
}
