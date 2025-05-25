package service

import (
	"github.com/notfrancois/filesystem-daemon/proto"
)

// Type aliases to make the service implementation cleaner
type (
	// Service interface
	FilesystemServiceServer              = proto.FilesystemServiceServer
	UnimplementedFilesystemServiceServer = proto.UnimplementedFilesystemServiceServer

	// Service request types
	ListRequest            = proto.ListRequest
	FileRequest            = proto.FileRequest
	CreateDirectoryRequest = proto.CreateDirectoryRequest
	DeleteRequest          = proto.DeleteRequest
	CopyRequest            = proto.CopyRequest
	MoveRequest            = proto.MoveRequest
	PathRequest            = proto.PathRequest
	SearchRequest          = proto.SearchRequest

	// Service response types
	ListResponse      = proto.ListResponse
	FileInfo          = proto.FileInfo
	FileItem          = proto.FileItem
	OperationResponse = proto.OperationResponse
	ExistsResponse    = proto.ExistsResponse
	SizeResponse      = proto.SizeResponse
	FileChunk         = proto.FileChunk

	// Streaming service interfaces
	FilesystemService_UploadFileServer   = proto.FilesystemService_UploadFileServer
	FilesystemService_DownloadFileServer = proto.FilesystemService_DownloadFileServer
)
