package service

import (
	"context"
	"fmt"
	"net/http" // For MIME type detection
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall" // For detailed file info
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// Import the generated protobuf code
	pb "github.com/notfrancois/filesystem-daemon/proto"
)

// FilesystemService implements the gRPC filesystem service
type FilesystemService struct {
	BaseDir   string
	Validator *AssetValidator
	pb.UnimplementedFilesystemServiceServer
}

// NewFilesystemService creates a new instance of the filesystem service
func NewFilesystemService(baseDir string) *FilesystemService {
	// Get configuration from environment
	maxSize := parseSize(os.Getenv("MAX_FILE_SIZE"))
	if maxSize == 0 {
		maxSize = 100 * 1024 * 1024 // Default 100MB
	}

	allowedExts := strings.Split(os.Getenv("ALLOWED_EXTENSIONS"), ",")
	if len(allowedExts) == 1 && allowedExts[0] == "" {
		allowedExts = []string{"jpg", "jpeg", "png", "gif", "svg", "css", "js", "html", "txt", "pdf"}
	}

	validator := NewAssetValidator(maxSize, allowedExts)

	return &FilesystemService{
		BaseDir:   baseDir,
		Validator: validator,
	}
}

func parseSize(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}

	if strings.HasSuffix(sizeStr, "MB") {
		if size, err := strconv.ParseInt(strings.TrimSuffix(sizeStr, "MB"), 10, 64); err == nil {
			return size * 1024 * 1024
		}
	} else if strings.HasSuffix(sizeStr, "GB") {
		if size, err := strconv.ParseInt(strings.TrimSuffix(sizeStr, "GB"), 10, 64); err == nil {
			return size * 1024 * 1024 * 1024
		}
	}

	// Try to parse as bytes
	if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		return size
	}

	return 0
}

// validatePath ensures the path is within the allowed base directory
// It resolves the full path and checks for directory traversal attacks
func (s *FilesystemService) validatePath(path string) (string, error) {
	// Normalize path separators for the current OS
	path = filepath.FromSlash(path)

	// Join with base directory to get absolute path
	fullPath := filepath.Join(s.BaseDir, path)

	// Get canonical path with symlinks resolved
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// If path doesn't exist yet, check its parent directory
		if os.IsNotExist(err) {
			parentDir := filepath.Dir(fullPath)
			realParentPath, err := filepath.EvalSymlinks(parentDir)
			if err != nil {
				return "", status.Errorf(codes.InvalidArgument, "Invalid path: %v", err)
			}
			// Check if parent is within base directory
			if !strings.HasPrefix(realParentPath, s.BaseDir) {
				return "", status.Errorf(codes.PermissionDenied, "Path is outside allowed directory")
			}
			return fullPath, nil
		}
		return "", status.Errorf(codes.InvalidArgument, "Invalid path: %v", err)
	}

	// Check if the path is within the allowed base directory
	if !strings.HasPrefix(realPath, s.BaseDir) {
		return "", status.Errorf(codes.PermissionDenied, "Path is outside allowed directory")
	}

	return fullPath, nil
}

// fileInfoToProto converts os.FileInfo to the protobuf FileInfo message
func fileInfoToProto(path string, info os.FileInfo) *pb.FileInfo {
	return &pb.FileInfo{
		Name:         info.Name(),
		Path:         path,
		IsDirectory:  info.IsDir(),
		Size:         info.Size(),
		ModifiedTime: info.ModTime().Unix(),
		Permissions:  fmt.Sprintf("%o", info.Mode().Perm()),
	}
}

// fileItemToProto converts os.FileInfo to the protobuf FileItem message (simpler than FileInfo)
func fileItemToProto(basePath string, info os.FileInfo) *pb.FileItem {
	return &pb.FileItem{
		Name:         info.Name(),
		Path:         filepath.Join(basePath, info.Name()),
		IsDirectory:  info.IsDir(),
		Size:         info.Size(),
		ModifiedTime: info.ModTime().Unix(),
		Permissions:  fmt.Sprintf("%o", info.Mode().Perm()),
		// Children and ParentPath will be available after proto regeneration
	}
}

// ListDirectory implements the ListDirectory RPC method
func (s *FilesystemService) ListDirectory(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if path exists and is a directory
	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "Directory does not exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to access directory: %v", err)
	}

	if !info.IsDir() {
		return nil, status.Errorf(codes.InvalidArgument, "Path is not a directory")
	}

	var response ListResponse

	// Handle recursive listing
	if req.Recursive {
		err = filepath.Walk(validPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip files with errors
			}

			// Skip the root directory itself
			if path == validPath {
				return nil
			}

			// If pattern is specified, check if it matches
			if req.Pattern != "" {
				matched, err := filepath.Match(req.Pattern, info.Name())
				if err != nil || !matched {
					return nil // Skip non-matching files
				}
			}

			// Get relative path from base
			relPath, err := filepath.Rel(s.BaseDir, path)
			if err != nil {
				return nil
			}

			item := fileItemToProto(filepath.Dir(relPath), info)
			response.Items = append(response.Items, item)
			return nil
		})

		if err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to list directory recursively: %v", err)
		}

		return &response, nil
	}

	// Non-recursive directory listing
	entries, err := os.ReadDir(validPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to read directory: %v", err)
	}

	// Get relative path from base for the request path
	relPath, err := filepath.Rel(s.BaseDir, validPath)
	if err != nil {
		relPath = req.Path // Use original path if relative path can't be determined
	}

	for _, entry := range entries {
		// If pattern is specified, check if it matches
		if req.Pattern != "" {
			matched, err := filepath.Match(req.Pattern, entry.Name())
			if err != nil || !matched {
				continue // Skip non-matching files
			}
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip entries with errors
		}

		item := fileItemToProto(relPath, info)
		response.Items = append(response.Items, item)
	}

	return &response, nil
}

// GetFileInfo implements the GetFileInfo RPC method
func (s *FilesystemService) GetFileInfo(ctx context.Context, req *FileRequest) (*FileInfo, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "File or directory does not exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to get file info: %v", err)
	}

	// Get relative path from base
	relPath, err := filepath.Rel(s.BaseDir, validPath)
	if err != nil {
		relPath = req.Path
	}

	// Create basic file info
	fileInfo := fileInfoToProto(relPath, info)

	// Additional file information (these might not be available on all platforms)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		fileInfo.CreationTime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec).Unix()
		fileInfo.AccessTime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec).Unix()
		fileInfo.Owner = strconv.FormatUint(uint64(stat.Uid), 10)
		fileInfo.Group = strconv.FormatUint(uint64(stat.Gid), 10)
	}

	// Determine MIME type for files (not directories)
	if !info.IsDir() {
		// Open file to detect MIME type
		file, err := os.Open(validPath)
		if err == nil {
			defer file.Close()

			// Read first 512 bytes for MIME detection
			buffer := make([]byte, 512)
			_, err := file.Read(buffer)
			if err == nil {
				fileInfo.MimeType = http.DetectContentType(buffer)
			}
		}
	}

	return fileInfo, nil
}

// Exists implements the Exists RPC method
func (s *FilesystemService) Exists(ctx context.Context, req *PathRequest) (*ExistsResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ExistsResponse{Exists: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "Failed to check path: %v", err)
	}

	return &ExistsResponse{
		Exists:      true,
		IsDirectory: info.IsDir(),
	}, nil
}

// GetHierarchy implements the GetHierarchy RPC method is defined in filesystem_hierarchy.go
