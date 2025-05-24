package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	
	pb "github.com/notfrancois/filesystem-daemon/proto"
)

// GetHierarchy implements the GetHierarchy RPC method
func (s *FilesystemService) GetHierarchy(ctx context.Context, req *pb.HierarchyRequest) (*pb.HierarchyResponse, error) {
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
	
	// Get relative path from base for the request path
	relPath, err := filepath.Rel(s.BaseDir, validPath)
	if err != nil {
		relPath = req.Path // Use original path if relative path can't be determined
	}
	
	// Build the root of our hierarchy
	rootItem := &pb.FileItem{
		Name:        filepath.Base(validPath),
		Path:        relPath,
		IsDirectory: true,
	}
	
	// Build hierarchy recursively with depth tracking
	err = s.buildHierarchy(ctx, rootItem, validPath, relPath, req.Pattern, 1, req.MaxDepth)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to build hierarchy: %v", err)
	}
	
	// Check if hierarchy was truncated due to max_depth
	truncated := false
	if req.MaxDepth > 0 {
		// Check if any directory at the max depth has contents
		truncated = s.checkTruncation(rootItem, 1, req.MaxDepth)
	}
	
	return &pb.HierarchyResponse{
		Root:      rootItem,
		Truncated: truncated,
	}, nil
}

// buildHierarchy recursively builds a directory hierarchy starting from a parent FileItem
func (s *FilesystemService) buildHierarchy(ctx context.Context, parent *pb.FileItem, fullPath, relPath, pattern string, currentDepth, maxDepth int32) error {
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue processing
	}

	// If max depth is specified and we've reached it, stop recursion
	if maxDepth > 0 && currentDepth > maxDepth {
		return nil
	}

	// Read directory entries
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	// Initialize children slice if not already initialized
	if parent.Children == nil {
		parent.Children = []*pb.FileItem{}
	}

	// Process each entry
	for _, entry := range entries {
		// If pattern is specified, check if it matches
		if pattern != "" {
			matched, err := filepath.Match(pattern, entry.Name())
			if err != nil || !matched {
				continue // Skip non-matching files
			}
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries with errors
		}

		// Create item
		item := &pb.FileItem{
			Name:         info.Name(),
			Path:         filepath.Join(relPath, info.Name()),
			IsDirectory:  info.IsDir(),
			Size:         info.Size(),
			ModifiedTime: info.ModTime().Unix(),
			Permissions:  fmt.Sprintf("%o", info.Mode().Perm()),
			ParentPath:   relPath,
		}

		// If directory, recursively process its contents
		if info.IsDir() {
			// Initialize children slice
			item.Children = []*pb.FileItem{}
			
			entryFullPath := filepath.Join(fullPath, info.Name())
			entryRelPath := filepath.Join(relPath, info.Name())
			
			// Recursively build hierarchy for this directory
			err = s.buildHierarchy(ctx, item, entryFullPath, entryRelPath, pattern, currentDepth+1, maxDepth)
			if err != nil {
				// Log error but continue with other entries
				fmt.Printf("Error processing directory %s: %v\n", entryFullPath, err)
			}
		}

		// Add to parent's children
		parent.Children = append(parent.Children, item)
	}

	return nil
}

// checkTruncation recursively checks if a hierarchy was truncated due to max depth
func (s *FilesystemService) checkTruncation(item *pb.FileItem, currentDepth, maxDepth int32) bool {
	// If at max depth and this is a directory, check if it has actual contents on disk
	if currentDepth == maxDepth && item.IsDirectory {
		// Construct the full path
		fullPath := filepath.Join(s.BaseDir, item.Path)
		
		// Check if the directory has any entries
		entries, err := os.ReadDir(fullPath)
		if err == nil && len(entries) > 0 {
			// Has contents but we didn't add them to our hierarchy due to depth limit
			return true
		}
	}

	// Check children recursively
	if currentDepth < maxDepth && item.Children != nil {
		for _, child := range item.Children {
			if child.IsDirectory {
				if s.checkTruncation(child, currentDepth+1, maxDepth) {
					return true
				}
			}
		}
	}

	return false
}
