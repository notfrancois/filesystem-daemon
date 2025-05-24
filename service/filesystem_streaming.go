package service

import (
	"io"
	"os"
	"path/filepath"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UploadFile implements the UploadFile RPC method (streaming from client)
func (s *FilesystemService) UploadFile(stream FilesystemService_UploadFileServer) error {
	var (
		fileData       *os.File
		currentPath    string
		bytesReceived  int64
	)
	
	// Cleanup function to close the file handle
	defer func() {
		if fileData != nil {
			fileData.Close()
		}
	}()
	
	for {
		// Receive file chunk from client
		chunk, err := stream.Recv()
		if err == io.EOF {
			// End of file reached
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "Error receiving file chunk: %v", err)
		}
		
		// If this is the first chunk, validate and open the file
		if fileData == nil {
			validPath, err := s.validatePath(chunk.FilePath)
			if err != nil {
				return err
			}
			
			// Create directory structure if needed
			dir := filepath.Dir(validPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return status.Errorf(codes.Internal, "Failed to create directory: %v", err)
			}
			
			// Open file for writing
			fileData, err = os.Create(validPath)
			if err != nil {
				return status.Errorf(codes.Internal, "Failed to create file: %v", err)
			}
			
			currentPath = validPath
		} else if chunk.FilePath != "" && currentPath != chunk.FilePath {
			// Path changed mid-stream - this is not allowed
			return status.Errorf(codes.InvalidArgument, "File path cannot change during upload")
		}
		
		// Write chunk to file
		n, err := fileData.Write(chunk.Content)
		if err != nil {
			return status.Errorf(codes.Internal, "Failed to write to file: %v", err)
		}
		
		bytesReceived += int64(n)
		
		// If this is the last chunk, break
		if chunk.IsLast {
			break
		}
	}
	
	// Close the file to ensure all data is written
	if fileData != nil {
		fileData.Close()
		fileData = nil
	}
	
	// Send success response
	return stream.SendAndClose(&OperationResponse{
		Success: true,
		Message: "File uploaded successfully",
	})
}

// DownloadFile implements the DownloadFile RPC method (streaming to client)
func (s *FilesystemService) DownloadFile(req *FileRequest, stream FilesystemService_DownloadFileServer) error {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return err
	}
	
	// Check if file exists and is not a directory
	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return status.Errorf(codes.NotFound, "File does not exist")
		}
		return status.Errorf(codes.Internal, "Failed to access file: %v", err)
	}
	
	if info.IsDir() {
		return status.Errorf(codes.InvalidArgument, "Path is a directory, not a file")
	}
	
	// Open the file
	file, err := os.Open(validPath)
	if err != nil {
		return status.Errorf(codes.Internal, "Failed to open file: %v", err)
	}
	defer file.Close()
	
	// Get relative path for client
	relPath, err := filepath.Rel(s.BaseDir, validPath)
	if err != nil {
		relPath = req.Path
	}
	
	// Send file in chunks
	buffer := make([]byte, 64*1024) // 64KB chunks
	offset := int64(0)
	
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			// End of file, send last chunk
			if n > 0 {
				chunk := &FileChunk{
					FilePath: relPath,
					Content:  buffer[:n],
					Offset:   offset,
					IsLast:   true,
				}
				
				if err := stream.Send(chunk); err != nil {
					return status.Errorf(codes.Internal, "Failed to send last chunk: %v", err)
				}
			}
			break
		}
		
		if err != nil {
			return status.Errorf(codes.Internal, "Error reading file: %v", err)
		}
		
		// Send chunk to client
		chunk := &FileChunk{
			FilePath: relPath,
			Content:  buffer[:n],
			Offset:   offset,
			IsLast:   false,
		}
		
		if err := stream.Send(chunk); err != nil {
			return status.Errorf(codes.Internal, "Failed to send chunk: %v", err)
		}
		
		offset += int64(n)
	}
	
	return nil
}
