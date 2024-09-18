// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.3
// source: proto/filesync.proto

package proto

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
	//	*FileSyncRequest_FileList
	Request isFileSyncRequest_Request `protobuf_oneof:"request"`
}

func (x *FileSyncRequest) Reset() {
	*x = FileSyncRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileSyncRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileSyncRequest) ProtoMessage() {}

func (x *FileSyncRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[0]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{0}
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

func (x *FileSyncRequest) GetFileList() *FileList {
	if x, ok := x.GetRequest().(*FileSyncRequest_FileList); ok {
		return x.FileList
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

type FileSyncRequest_FileList struct {
	FileList *FileList `protobuf:"bytes,6,opt,name=file_list,json=fileList,proto3,oneof"`
}

func (*FileSyncRequest_FileChunk) isFileSyncRequest_Request() {}

func (*FileSyncRequest_FileDelete) isFileSyncRequest_Request() {}

func (*FileSyncRequest_FileRename) isFileSyncRequest_Request() {}

func (*FileSyncRequest_Ack) isFileSyncRequest_Request() {}

func (*FileSyncRequest_Poll) isFileSyncRequest_Request() {}

func (*FileSyncRequest_FileList) isFileSyncRequest_Request() {}

type StateReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StateReq) Reset() {
	*x = StateReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StateReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StateReq) ProtoMessage() {}

func (x *StateReq) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StateReq.ProtoReflect.Descriptor instead.
func (*StateReq) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{1}
}

type MetaDataReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
}

func (x *MetaDataReq) Reset() {
	*x = MetaDataReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetaDataReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetaDataReq) ProtoMessage() {}

func (x *MetaDataReq) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetaDataReq.ProtoReflect.Descriptor instead.
func (*MetaDataReq) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{2}
}

func (x *MetaDataReq) GetFileName() string {
	if x != nil {
		return x.FileName
	}
	return ""
}

type FileMetaData struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName    string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
	ChunkNumber int64  `protobuf:"varint,2,opt,name=chunk_number,json=chunkNumber,proto3" json:"chunk_number,omitempty"`
	ChunkSize   int64  `protobuf:"varint,3,opt,name=chunk_size,json=chunkSize,proto3" json:"chunk_size,omitempty"`
	ChunkHash   string `protobuf:"bytes,4,opt,name=chunk_hash,json=chunkHash,proto3" json:"chunk_hash,omitempty"`
	Message     string `protobuf:"bytes,5,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *FileMetaData) Reset() {
	*x = FileMetaData{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileMetaData) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileMetaData) ProtoMessage() {}

func (x *FileMetaData) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileMetaData.ProtoReflect.Descriptor instead.
func (*FileMetaData) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{3}
}

func (x *FileMetaData) GetFileName() string {
	if x != nil {
		return x.FileName
	}
	return ""
}

func (x *FileMetaData) GetChunkNumber() int64 {
	if x != nil {
		return x.ChunkNumber
	}
	return 0
}

func (x *FileMetaData) GetChunkSize() int64 {
	if x != nil {
		return x.ChunkSize
	}
	return 0
}

func (x *FileMetaData) GetChunkHash() string {
	if x != nil {
		return x.ChunkHash
	}
	return ""
}

func (x *FileMetaData) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type StateRes struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message []string `protobuf:"bytes,1,rep,name=message,proto3" json:"message,omitempty"` // this is only for a list of files if State is supplied in the StateReq
}

func (x *StateRes) Reset() {
	*x = StateRes{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StateRes) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StateRes) ProtoMessage() {}

func (x *StateRes) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StateRes.ProtoReflect.Descriptor instead.
func (*StateRes) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{4}
}

func (x *StateRes) GetMessage() []string {
	if x != nil {
		return x.Message
	}
	return nil
}

type FileChunk struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName    string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
	ChunkData   []byte `protobuf:"bytes,2,opt,name=chunk_data,json=chunkData,proto3" json:"chunk_data,omitempty"`
	ChunkNumber int32  `protobuf:"varint,3,opt,name=chunk_number,json=chunkNumber,proto3" json:"chunk_number,omitempty"`
	ChunkSize   int64  `protobuf:"varint,4,opt,name=chunk_size,json=chunkSize,proto3" json:"chunk_size,omitempty"`
	TotalChunks int32  `protobuf:"varint,5,opt,name=total_chunks,json=totalChunks,proto3" json:"total_chunks,omitempty"`
}

func (x *FileChunk) Reset() {
	*x = FileChunk{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileChunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileChunk) ProtoMessage() {}

func (x *FileChunk) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[5]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{5}
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

func (x *FileChunk) GetChunkNumber() int32 {
	if x != nil {
		return x.ChunkNumber
	}
	return 0
}

func (x *FileChunk) GetChunkSize() int64 {
	if x != nil {
		return x.ChunkSize
	}
	return 0
}

func (x *FileChunk) GetTotalChunks() int32 {
	if x != nil {
		return x.TotalChunks
	}
	return 0
}

type MetaDataChunk struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FileName    string `protobuf:"bytes,1,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
	ChunkData   []byte `protobuf:"bytes,2,opt,name=chunk_data,json=chunkData,proto3" json:"chunk_data,omitempty"`
	ChunkNumber int64  `protobuf:"varint,3,opt,name=chunk_number,json=chunkNumber,proto3" json:"chunk_number,omitempty"`
	ChunkSize   int64  `protobuf:"varint,4,opt,name=chunk_size,json=chunkSize,proto3" json:"chunk_size,omitempty"`
	TotalChunks int32  `protobuf:"varint,5,opt,name=total_chunks,json=totalChunks,proto3" json:"total_chunks,omitempty"`
}

func (x *MetaDataChunk) Reset() {
	*x = MetaDataChunk{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetaDataChunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetaDataChunk) ProtoMessage() {}

func (x *MetaDataChunk) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetaDataChunk.ProtoReflect.Descriptor instead.
func (*MetaDataChunk) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{6}
}

func (x *MetaDataChunk) GetFileName() string {
	if x != nil {
		return x.FileName
	}
	return ""
}

func (x *MetaDataChunk) GetChunkData() []byte {
	if x != nil {
		return x.ChunkData
	}
	return nil
}

func (x *MetaDataChunk) GetChunkNumber() int64 {
	if x != nil {
		return x.ChunkNumber
	}
	return 0
}

func (x *MetaDataChunk) GetChunkSize() int64 {
	if x != nil {
		return x.ChunkSize
	}
	return 0
}

func (x *MetaDataChunk) GetTotalChunks() int32 {
	if x != nil {
		return x.TotalChunks
	}
	return 0
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
		mi := &file_proto_filesync_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileDelete) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileDelete) ProtoMessage() {}

func (x *FileDelete) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[7]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{7}
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
		mi := &file_proto_filesync_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileRename) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileRename) ProtoMessage() {}

func (x *FileRename) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[8]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{8}
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

	Message     string   `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	Filestosend []string `protobuf:"bytes,2,rep,name=filestosend,proto3" json:"filestosend,omitempty"`
	Filedeleted string   `protobuf:"bytes,3,opt,name=filedeleted,proto3" json:"filedeleted,omitempty"`
}

func (x *FileSyncResponse) Reset() {
	*x = FileSyncResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileSyncResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileSyncResponse) ProtoMessage() {}

func (x *FileSyncResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[9]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{9}
}

func (x *FileSyncResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *FileSyncResponse) GetFilestosend() []string {
	if x != nil {
		return x.Filestosend
	}
	return nil
}

func (x *FileSyncResponse) GetFiledeleted() string {
	if x != nil {
		return x.Filedeleted
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
		mi := &file_proto_filesync_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Acknowledgment) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Acknowledgment) ProtoMessage() {}

func (x *Acknowledgment) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[10]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{10}
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
		mi := &file_proto_filesync_proto_msgTypes[11]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Poll) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Poll) ProtoMessage() {}

func (x *Poll) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[11]
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
	return file_proto_filesync_proto_rawDescGZIP(), []int{11}
}

func (x *Poll) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type FileList struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Files []string `protobuf:"bytes,1,rep,name=files,proto3" json:"files,omitempty"`
}

func (x *FileList) Reset() {
	*x = FileList{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_filesync_proto_msgTypes[12]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FileList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileList) ProtoMessage() {}

func (x *FileList) ProtoReflect() protoreflect.Message {
	mi := &file_proto_filesync_proto_msgTypes[12]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileList.ProtoReflect.Descriptor instead.
func (*FileList) Descriptor() ([]byte, []int) {
	return file_proto_filesync_proto_rawDescGZIP(), []int{12}
}

func (x *FileList) GetFiles() []string {
	if x != nil {
		return x.Files
	}
	return nil
}

var File_proto_filesync_proto protoreflect.FileDescriptor

var file_proto_filesync_proto_rawDesc = []byte{
	0x0a, 0x14, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x79, 0x6e, 0x63,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x95, 0x02, 0x0a, 0x0f, 0x46, 0x69, 0x6c, 0x65, 0x53,
	0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2b, 0x0a, 0x0a, 0x66, 0x69,
	0x6c, 0x65, 0x5f, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0a,
	0x2e, 0x46, 0x69, 0x6c, 0x65, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x48, 0x00, 0x52, 0x09, 0x66, 0x69,
	0x6c, 0x65, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x12, 0x2e, 0x0a, 0x0b, 0x66, 0x69, 0x6c, 0x65, 0x5f,
	0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x46,
	0x69, 0x6c, 0x65, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x48, 0x00, 0x52, 0x0a, 0x66, 0x69, 0x6c,
	0x65, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x12, 0x2e, 0x0a, 0x0b, 0x66, 0x69, 0x6c, 0x65, 0x5f,
	0x72, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x46,
	0x69, 0x6c, 0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x48, 0x00, 0x52, 0x0a, 0x66, 0x69, 0x6c,
	0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x23, 0x0a, 0x03, 0x61, 0x63, 0x6b, 0x18, 0x04,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x41, 0x63, 0x6b, 0x6e, 0x6f, 0x77, 0x6c, 0x65, 0x64,
	0x67, 0x6d, 0x65, 0x6e, 0x74, 0x48, 0x00, 0x52, 0x03, 0x61, 0x63, 0x6b, 0x12, 0x1b, 0x0a, 0x04,
	0x70, 0x6f, 0x6c, 0x6c, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x05, 0x2e, 0x50, 0x6f, 0x6c,
	0x6c, 0x48, 0x00, 0x52, 0x04, 0x70, 0x6f, 0x6c, 0x6c, 0x12, 0x28, 0x0a, 0x09, 0x66, 0x69, 0x6c,
	0x65, 0x5f, 0x6c, 0x69, 0x73, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x46,
	0x69, 0x6c, 0x65, 0x4c, 0x69, 0x73, 0x74, 0x48, 0x00, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4c,
	0x69, 0x73, 0x74, 0x42, 0x09, 0x0a, 0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x0a,
	0x0a, 0x08, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x22, 0x2a, 0x0a, 0x0b, 0x4d, 0x65,
	0x74, 0x61, 0x44, 0x61, 0x74, 0x61, 0x52, 0x65, 0x71, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c,
	0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69,
	0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x22, 0xa6, 0x01, 0x0a, 0x0c, 0x46, 0x69, 0x6c, 0x65, 0x4d,
	0x65, 0x74, 0x61, 0x44, 0x61, 0x74, 0x61, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f,
	0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65,
	0x4e, 0x61, 0x6d, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x6e, 0x75,
	0x6d, 0x62, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0b, 0x63, 0x68, 0x75, 0x6e,
	0x6b, 0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b,
	0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x63, 0x68, 0x75,
	0x6e, 0x6b, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f,
	0x68, 0x61, 0x73, 0x68, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e,
	0x6b, 0x48, 0x61, 0x73, 0x68, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22,
	0x24, 0x0a, 0x08, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0xac, 0x01, 0x0a, 0x09, 0x46, 0x69, 0x6c, 0x65, 0x43, 0x68,
	0x75, 0x6e, 0x6b, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65,
	0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x44, 0x61, 0x74, 0x61, 0x12,
	0x21, 0x0a, 0x0c, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0b, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x4e, 0x75, 0x6d, 0x62,
	0x65, 0x72, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x53, 0x69, 0x7a,
	0x65, 0x12, 0x21, 0x0a, 0x0c, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x5f, 0x63, 0x68, 0x75, 0x6e, 0x6b,
	0x73, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0b, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x43, 0x68,
	0x75, 0x6e, 0x6b, 0x73, 0x22, 0xb0, 0x01, 0x0a, 0x0d, 0x4d, 0x65, 0x74, 0x61, 0x44, 0x61, 0x74,
	0x61, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4e,
	0x61, 0x6d, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x64, 0x61, 0x74,
	0x61, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x44, 0x61,
	0x74, 0x61, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x6e, 0x75, 0x6d, 0x62,
	0x65, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0b, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x4e,
	0x75, 0x6d, 0x62, 0x65, 0x72, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x5f, 0x73,
	0x69, 0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x63, 0x68, 0x75, 0x6e, 0x6b,
	0x53, 0x69, 0x7a, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x5f, 0x63, 0x68,
	0x75, 0x6e, 0x6b, 0x73, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0b, 0x74, 0x6f, 0x74, 0x61,
	0x6c, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x73, 0x22, 0x29, 0x0a, 0x0a, 0x46, 0x69, 0x6c, 0x65, 0x44,
	0x65, 0x6c, 0x65, 0x74, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x6e, 0x61,
	0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x4e, 0x61,
	0x6d, 0x65, 0x22, 0x42, 0x0a, 0x0a, 0x46, 0x69, 0x6c, 0x65, 0x52, 0x65, 0x6e, 0x61, 0x6d, 0x65,
	0x12, 0x19, 0x0a, 0x08, 0x6f, 0x6c, 0x64, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x6f, 0x6c, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x6e,
	0x65, 0x77, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6e,
	0x65, 0x77, 0x4e, 0x61, 0x6d, 0x65, 0x22, 0x70, 0x0a, 0x10, 0x46, 0x69, 0x6c, 0x65, 0x53, 0x79,
	0x6e, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x74, 0x6f, 0x73,
	0x65, 0x6e, 0x64, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0b, 0x66, 0x69, 0x6c, 0x65, 0x73,
	0x74, 0x6f, 0x73, 0x65, 0x6e, 0x64, 0x12, 0x20, 0x0a, 0x0b, 0x66, 0x69, 0x6c, 0x65, 0x64, 0x65,
	0x6c, 0x65, 0x74, 0x65, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x66, 0x69, 0x6c,
	0x65, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x64, 0x22, 0x2a, 0x0a, 0x0e, 0x41, 0x63, 0x6b, 0x6e,
	0x6f, 0x77, 0x6c, 0x65, 0x64, 0x67, 0x6d, 0x65, 0x6e, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x22, 0x20, 0x0a, 0x04, 0x50, 0x6f, 0x6c, 0x6c, 0x12, 0x18, 0x0a, 0x07,
	0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x20, 0x0a, 0x08, 0x46, 0x69, 0x6c, 0x65, 0x4c, 0x69,
	0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x32, 0xcb, 0x01, 0x0a, 0x0f, 0x46, 0x69, 0x6c,
	0x65, 0x53, 0x79, 0x6e, 0x63, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x34, 0x0a, 0x09,
	0x53, 0x79, 0x6e, 0x63, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x12, 0x10, 0x2e, 0x46, 0x69, 0x6c, 0x65,
	0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x11, 0x2e, 0x46, 0x69,
	0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x28, 0x01,
	0x30, 0x01, 0x12, 0x1f, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x09, 0x2e, 0x53, 0x74,
	0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x1a, 0x09, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x65,
	0x73, 0x30, 0x01, 0x12, 0x34, 0x0a, 0x0b, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x46, 0x69, 0x6c,
	0x65, 0x73, 0x12, 0x0e, 0x2e, 0x4d, 0x65, 0x74, 0x61, 0x44, 0x61, 0x74, 0x61, 0x43, 0x68, 0x75,
	0x6e, 0x6b, 0x1a, 0x11, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x28, 0x01, 0x30, 0x01, 0x12, 0x2b, 0x0a, 0x08, 0x4d, 0x65, 0x74,
	0x61, 0x44, 0x61, 0x74, 0x61, 0x12, 0x0c, 0x2e, 0x4d, 0x65, 0x74, 0x61, 0x44, 0x61, 0x74, 0x61,
	0x52, 0x65, 0x71, 0x1a, 0x0d, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4d, 0x65, 0x74, 0x61, 0x44, 0x61,
	0x74, 0x61, 0x28, 0x01, 0x30, 0x01, 0x42, 0x0e, 0x5a, 0x0c, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f,
	0x3b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_proto_filesync_proto_rawDescOnce sync.Once
	file_proto_filesync_proto_rawDescData = file_proto_filesync_proto_rawDesc
)

func file_proto_filesync_proto_rawDescGZIP() []byte {
	file_proto_filesync_proto_rawDescOnce.Do(func() {
		file_proto_filesync_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_filesync_proto_rawDescData)
	})
	return file_proto_filesync_proto_rawDescData
}

var file_proto_filesync_proto_msgTypes = make([]protoimpl.MessageInfo, 13)
var file_proto_filesync_proto_goTypes = []any{
	(*FileSyncRequest)(nil),  // 0: FileSyncRequest
	(*StateReq)(nil),         // 1: StateReq
	(*MetaDataReq)(nil),      // 2: MetaDataReq
	(*FileMetaData)(nil),     // 3: FileMetaData
	(*StateRes)(nil),         // 4: StateRes
	(*FileChunk)(nil),        // 5: FileChunk
	(*MetaDataChunk)(nil),    // 6: MetaDataChunk
	(*FileDelete)(nil),       // 7: FileDelete
	(*FileRename)(nil),       // 8: FileRename
	(*FileSyncResponse)(nil), // 9: FileSyncResponse
	(*Acknowledgment)(nil),   // 10: Acknowledgment
	(*Poll)(nil),             // 11: Poll
	(*FileList)(nil),         // 12: FileList
}
var file_proto_filesync_proto_depIdxs = []int32{
	5,  // 0: FileSyncRequest.file_chunk:type_name -> FileChunk
	7,  // 1: FileSyncRequest.file_delete:type_name -> FileDelete
	8,  // 2: FileSyncRequest.file_rename:type_name -> FileRename
	10, // 3: FileSyncRequest.ack:type_name -> Acknowledgment
	11, // 4: FileSyncRequest.poll:type_name -> Poll
	12, // 5: FileSyncRequest.file_list:type_name -> FileList
	0,  // 6: FileSyncService.SyncFiles:input_type -> FileSyncRequest
	1,  // 7: FileSyncService.State:input_type -> StateReq
	6,  // 8: FileSyncService.ModifyFiles:input_type -> MetaDataChunk
	2,  // 9: FileSyncService.MetaData:input_type -> MetaDataReq
	9,  // 10: FileSyncService.SyncFiles:output_type -> FileSyncResponse
	4,  // 11: FileSyncService.State:output_type -> StateRes
	9,  // 12: FileSyncService.ModifyFiles:output_type -> FileSyncResponse
	3,  // 13: FileSyncService.MetaData:output_type -> FileMetaData
	10, // [10:14] is the sub-list for method output_type
	6,  // [6:10] is the sub-list for method input_type
	6,  // [6:6] is the sub-list for extension type_name
	6,  // [6:6] is the sub-list for extension extendee
	0,  // [0:6] is the sub-list for field type_name
}

func init() { file_proto_filesync_proto_init() }
func file_proto_filesync_proto_init() {
	if File_proto_filesync_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_filesync_proto_msgTypes[0].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*StateReq); i {
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
		file_proto_filesync_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*MetaDataReq); i {
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
		file_proto_filesync_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*FileMetaData); i {
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
		file_proto_filesync_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*StateRes); i {
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
		file_proto_filesync_proto_msgTypes[5].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[6].Exporter = func(v any, i int) any {
			switch v := v.(*MetaDataChunk); i {
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
		file_proto_filesync_proto_msgTypes[7].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[8].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[9].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[10].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[11].Exporter = func(v any, i int) any {
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
		file_proto_filesync_proto_msgTypes[12].Exporter = func(v any, i int) any {
			switch v := v.(*FileList); i {
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
	file_proto_filesync_proto_msgTypes[0].OneofWrappers = []any{
		(*FileSyncRequest_FileChunk)(nil),
		(*FileSyncRequest_FileDelete)(nil),
		(*FileSyncRequest_FileRename)(nil),
		(*FileSyncRequest_Ack)(nil),
		(*FileSyncRequest_Poll)(nil),
		(*FileSyncRequest_FileList)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_filesync_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   13,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_filesync_proto_goTypes,
		DependencyIndexes: file_proto_filesync_proto_depIdxs,
		MessageInfos:      file_proto_filesync_proto_msgTypes,
	}.Build()
	File_proto_filesync_proto = out.File
	file_proto_filesync_proto_rawDesc = nil
	file_proto_filesync_proto_goTypes = nil
	file_proto_filesync_proto_depIdxs = nil
}
