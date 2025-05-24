package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateDirectory implements the CreateDirectory RPC method
func (s *FilesystemService) CreateDirectory(ctx context.Context, req *CreateDirectoryRequest) (*OperationResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if directory already exists
	if _, err := os.Stat(validPath); err == nil {
		return &OperationResponse{
			Success: false,
			Error:   "Directory already exists",
		}, nil
	}

	// Create directory with specified permissions
	var perm os.FileMode = 0755 // Default permissions
	if req.Permissions > 0 {
		perm = os.FileMode(req.Permissions)
	}

	if err := os.MkdirAll(validPath, perm); err != nil {
		return &OperationResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &OperationResponse{
		Success: true,
		Message: "Directory created successfully",
	}, nil
}

// Delete implements the Delete RPC method
func (s *FilesystemService) Delete(ctx context.Context, req *DeleteRequest) (*OperationResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if path exists
	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &OperationResponse{
				Success: false,
				Error:   "File or directory does not exist",
			}, nil
		}
		return &OperationResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// If it's a directory, check recursive flag
	if info.IsDir() && !req.Recursive {
		// Check if directory is empty
		entries, err := os.ReadDir(validPath)
		if err != nil {
			return &OperationResponse{
				Success: false,
				Error:   "Failed to read directory: " + err.Error(),
			}, nil
		}

		if len(entries) > 0 {
			return &OperationResponse{
				Success: false,
				Error:   "Directory is not empty and recursive flag is not set",
			}, nil
		}

		// Directory is empty, delete it
		if err := os.Remove(validPath); err != nil {
			return &OperationResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	} else if info.IsDir() && req.Recursive {
		// Recursive delete for directory
		if err := os.RemoveAll(validPath); err != nil {
			return &OperationResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	} else {
		// Delete file
		if err := os.Remove(validPath); err != nil {
			return &OperationResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	}

	return &OperationResponse{
		Success: true,
		Message: "Deleted successfully",
	}, nil
}

// Copy implements the Copy RPC method
func (s *FilesystemService) Copy(ctx context.Context, req *CopyRequest) (*OperationResponse, error) {
	validSourcePath, err := s.validatePath(req.Source)
	if err != nil {
		return nil, err
	}

	validDestPath, err := s.validatePath(req.Destination)
	if err != nil {
		return nil, err
	}

	// Check if source exists
	srcInfo, err := os.Stat(validSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &OperationResponse{
				Success: false,
				Error:   "Source does not exist",
			}, nil
		}
		return &OperationResponse{
			Success: false,
			Error:   "Failed to access source: " + err.Error(),
		}, nil
	}

	// Check if destination already exists
	if _, err := os.Stat(validDestPath); err == nil && !req.Overwrite {
		return &OperationResponse{
			Success: false,
			Error:   "Destination already exists and overwrite is not enabled",
		}, nil
	}

	// Handle directory copy
	if srcInfo.IsDir() {
		err = copyDir(validSourcePath, validDestPath)
		if err != nil {
			return &OperationResponse{
				Success: false,
				Error:   "Failed to copy directory: " + err.Error(),
			}, nil
		}
	} else {
		// Create destination directory if it doesn't exist
		destDir := filepath.Dir(validDestPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return &OperationResponse{
				Success: false,
				Error:   "Failed to create destination directory: " + err.Error(),
			}, nil
		}

		// Copy file
		err = copyFile(validSourcePath, validDestPath)
		if err != nil {
			return &OperationResponse{
				Success: false,
				Error:   "Failed to copy file: " + err.Error(),
			}, nil
		}
	}

	return &OperationResponse{
		Success: true,
		Message: "Copy completed successfully",
	}, nil
}

// Move implements the Move RPC method
func (s *FilesystemService) Move(ctx context.Context, req *MoveRequest) (*OperationResponse, error) {
	validSourcePath, err := s.validatePath(req.Source)
	if err != nil {
		return nil, err
	}

	validDestPath, err := s.validatePath(req.Destination)
	if err != nil {
		return nil, err
	}

	// Check if source exists
	if _, err := os.Stat(validSourcePath); err != nil {
		if os.IsNotExist(err) {
			return &OperationResponse{
				Success: false,
				Error:   "Source does not exist",
			}, nil
		}
		return &OperationResponse{
			Success: false,
			Error:   "Failed to access source: " + err.Error(),
		}, nil
	}

	// Check if destination already exists
	if _, err := os.Stat(validDestPath); err == nil && !req.Overwrite {
		return &OperationResponse{
			Success: false,
			Error:   "Destination already exists and overwrite is not enabled",
		}, nil
	}

	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(validDestPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return &OperationResponse{
			Success: false,
			Error:   "Failed to create destination directory: " + err.Error(),
		}, nil
	}

	// Move/rename the file or directory
	if err := os.Rename(validSourcePath, validDestPath); err != nil {
		return &OperationResponse{
			Success: false,
			Error:   "Failed to move: " + err.Error(),
		}, nil
	}

	return &OperationResponse{
		Success: true,
		Message: "Move completed successfully",
	}, nil
}

// GetDirectorySize implements the GetDirectorySize RPC method
func (s *FilesystemService) GetDirectorySize(ctx context.Context, req *PathRequest) (*SizeResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if path exists
	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "Path does not exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to access path: %v", err)
	}

	// If it's a file, return its size directly
	if !info.IsDir() {
		return &SizeResponse{Size: info.Size()}, nil
	}

	// For a directory, calculate total size recursively
	var totalSize int64
	err = filepath.Walk(validPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to calculate directory size: %v", err)
	}

	return &SizeResponse{Size: totalSize}, nil
}

// Search implements the Search RPC method
func (s *FilesystemService) Search(ctx context.Context, req *SearchRequest) (*ListResponse, error) {
	validPath, err := s.validatePath(req.BasePath)
	if err != nil {
		return nil, err
	}

	// Check if base path exists and is a directory
	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "Base directory does not exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to access directory: %v", err)
	}

	if !info.IsDir() {
		return nil, status.Errorf(codes.InvalidArgument, "Base path is not a directory")
	}

	var response ListResponse
	var count int32

	// Walk through directory recursively
	err = filepath.Walk(validPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip root directory
		if path == validPath {
			return nil
		}

		// If max results is specified and reached, stop search
		if req.MaxResults > 0 && count >= req.MaxResults {
			return filepath.SkipDir
		}

		// Skip directories if files_only is true
		if info.IsDir() && req.FilesOnly {
			return nil
		}

		// Skip files if directories_only is true
		if !info.IsDir() && req.DirectoriesOnly {
			return nil
		}

		// Skip non-recursive search
		if !req.Recursive && filepath.Dir(path) != validPath {
			return nil
		}

		// Apply pattern matching
		if req.Pattern != "" {
			var matched bool
			if req.CaseSensitive {
				matched, _ = filepath.Match(req.Pattern, info.Name())
			} else {
				matched, _ = filepath.Match(strings.ToLower(req.Pattern), strings.ToLower(info.Name()))
			}

			if !matched {
				return nil
			}
		}

		// Get relative path from base directory
		relPath, err := filepath.Rel(s.BaseDir, path)
		if err != nil {
			return nil
		}

		// Add to results
		item := fileItemToProto(filepath.Dir(relPath), info)
		response.Items = append(response.Items, item)
		count++

		return nil
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Search failed: %v", err)
	}

	return &response, nil
}

// Helper functions for file operations

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy content
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Get source file mode
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Set same permissions
	return os.Chmod(dst, sourceInfo.Mode())
}

func copyDir(src, dst string) error {
	// Get source info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}

		if entryInfo.IsDir() {
			// Recursive copy for directories
			if err = copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err = copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
