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
	HierarchyRequest       = proto.HierarchyRequest

	// New editor request types
	OpenFileRequest         = proto.OpenFileRequest
	CloseFileRequest        = proto.CloseFileRequest
	WriteFileContentRequest = proto.WriteFileContentRequest
	GetFileLinesRequest     = proto.GetFileLinesRequest
	UpdateFileLinesRequest  = proto.UpdateFileLinesRequest
	LockFileRequest         = proto.LockFileRequest
	UnlockFileRequest       = proto.UnlockFileRequest

	// Service response types
	ListResponse      = proto.ListResponse
	FileInfo          = proto.FileInfo
	FileItem          = proto.FileItem
	OperationResponse = proto.OperationResponse
	ExistsResponse    = proto.ExistsResponse
	SizeResponse      = proto.SizeResponse
	FileChunk         = proto.FileChunk
	HierarchyResponse = proto.HierarchyResponse

	// New editor response types
	OpenFileResponse    = proto.OpenFileResponse
	FileContentResponse = proto.FileContentResponse
	FileLinesResponse   = proto.FileLinesResponse
	LockFileResponse    = proto.LockFileResponse
	FileLine            = proto.FileLine
	LineUpdate          = proto.LineUpdate

	// Enums
	FileOpenMode  = proto.FileOpenMode
	LineOperation = proto.LineOperation
	LockType      = proto.LockType

	// Streaming service interfaces
	FilesystemService_UploadFileServer   = proto.FilesystemService_UploadFileServer
	FilesystemService_DownloadFileServer = proto.FilesystemService_DownloadFileServer
)
