package service

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	pb "github.com/notfrancois/filesystem-daemon/proto"
)

// FileSession represents an open file session
type FileSession struct {
	Handle     string
	Path       string
	Mode       pb.FileOpenMode
	File       *os.File
	LockID     string
	CreatedAt  time.Time
	LastAccess time.Time
	HasChanges bool
	BackupPath string
}

// FileLock represents a file lock
type FileLock struct {
	LockID    string
	Path      string
	Type      pb.LockType
	ExpiresAt time.Time
	Owner     string // For future multi-user support
}

// FileEditor manages file editing sessions and locks
type FileEditor struct {
	sessions map[string]*FileSession
	locks    map[string]*FileLock
	mu       sync.RWMutex
}

// Global file editor instance
var fileEditor = &FileEditor{
	sessions: make(map[string]*FileSession),
	locks:    make(map[string]*FileLock),
}

// generateHandle creates a unique handle for file sessions
func generateHandle() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateLockID creates a unique lock identifier
func generateLockID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return "lock_" + hex.EncodeToString(bytes)
}

// cleanupExpiredLocks removes expired locks
func (fe *FileEditor) cleanupExpiredLocks() {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	now := time.Now()
	for path, lock := range fe.locks {
		if now.After(lock.ExpiresAt) {
			delete(fe.locks, path)
		}
	}
}

// OpenFile implements the OpenFile RPC method
func (s *FilesystemService) OpenFile(ctx context.Context, req *pb.OpenFileRequest) (*pb.OpenFileResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if file exists or if we should create it
	_, err = os.Stat(validPath)
	if os.IsNotExist(err) && !req.CreateIfNotExists {
		return &pb.OpenFileResponse{
			Success: false,
			Error:   "File does not exist",
		}, nil
	}

	// Determine file mode flags
	var flags int
	switch req.Mode {
	case pb.FileOpenMode_READ_ONLY:
		flags = os.O_RDONLY
	case pb.FileOpenMode_WRITE_ONLY:
		flags = os.O_WRONLY
		if req.CreateIfNotExists {
			flags |= os.O_CREATE
		}
	case pb.FileOpenMode_READ_WRITE:
		flags = os.O_RDWR
		if req.CreateIfNotExists {
			flags |= os.O_CREATE
		}
	default:
		return &pb.OpenFileResponse{
			Success: false,
			Error:   "Invalid file open mode",
		}, nil
	}

	// Create directory if it doesn't exist
	if req.CreateIfNotExists {
		if err := os.MkdirAll(filepath.Dir(validPath), 0755); err != nil {
			return &pb.OpenFileResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to create directory: %v", err),
			}, nil
		}
	}

	// Open the file
	file, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		return &pb.OpenFileResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to open file: %v", err),
		}, nil
	}

	// Generate session handle
	handle := generateHandle()

	// Handle exclusive locking if requested
	var lockID string
	if req.ExclusiveLock {
		lockReq := &pb.LockFileRequest{
			Path:           req.Path,
			LockType:       pb.LockType_EXCLUSIVE,
			TimeoutSeconds: 300, // 5 minutes default
		}

		lockResp, err := s.LockFile(ctx, lockReq)
		if err != nil {
			file.Close()
			return nil, err
		}

		if !lockResp.Success {
			file.Close()
			return &pb.OpenFileResponse{
				Success: false,
				Error:   lockResp.Error,
			}, nil
		}

		lockID = lockResp.LockId
	}

	// Get file info
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return &pb.OpenFileResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to get file info: %v", err),
		}, nil
	}

	// Create session
	session := &FileSession{
		Handle:     handle,
		Path:       validPath,
		Mode:       req.Mode,
		File:       file,
		LockID:     lockID,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		HasChanges: false,
	}

	// Store session
	fileEditor.mu.Lock()
	fileEditor.sessions[handle] = session
	fileEditor.mu.Unlock()

	// Convert file info to proto
	fileInfo := &pb.FileInfo{
		Name:         info.Name(),
		Path:         req.Path,
		IsDirectory:  info.IsDir(),
		Size:         info.Size(),
		ModifiedTime: info.ModTime().Unix(),
		Permissions:  fmt.Sprintf("%o", info.Mode().Perm()),
	}

	return &pb.OpenFileResponse{
		Success:    true,
		FileHandle: handle,
		FileInfo:   fileInfo,
		LockId:     lockID,
	}, nil
}

// CloseFile implements the CloseFile RPC method
func (s *FilesystemService) CloseFile(ctx context.Context, req *pb.CloseFileRequest) (*pb.OperationResponse, error) {
	fileEditor.mu.Lock()
	defer fileEditor.mu.Unlock()

	session, exists := fileEditor.sessions[req.FileHandle]
	if !exists {
		return &pb.OperationResponse{
			Success: false,
			Error:   "Invalid file handle",
		}, nil
	}

	// Close the file
	if err := session.File.Close(); err != nil {
		return &pb.OperationResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to close file: %v", err),
		}, nil
	}

	// Release lock if exists
	if session.LockID != "" {
		unlockReq := &pb.UnlockFileRequest{
			Path:   session.Path,
			LockId: session.LockID,
		}
		s.UnlockFile(ctx, unlockReq)
	}

	// Remove session
	delete(fileEditor.sessions, req.FileHandle)

	message := "File closed successfully"
	if session.HasChanges && req.SaveChanges {
		message = "File closed and changes saved successfully"
	}

	return &pb.OperationResponse{
		Success: true,
		Message: message,
	}, nil
}

// ReadFileContent implements the ReadFileContent RPC method
func (s *FilesystemService) ReadFileContent(ctx context.Context, req *pb.FileRequest) (*pb.FileContentResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Read file content
	content, err := os.ReadFile(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &pb.FileContentResponse{
				Success: false,
				Error:   "File does not exist",
			}, nil
		}
		return &pb.FileContentResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read file: %v", err),
		}, nil
	}

	// Detect encoding (simple check for UTF-8)
	encoding := "utf-8"
	if !utf8.Valid(content) {
		encoding = "binary"
	}

	// Convert to string (may not be perfect for binary files)
	contentStr := string(content)

	// Count lines
	lineCount := strings.Count(contentStr, "\n") + 1
	if len(contentStr) == 0 {
		lineCount = 0
	}

	return &pb.FileContentResponse{
		Success:   true,
		Content:   contentStr,
		Encoding:  encoding,
		LineCount: int32(lineCount),
		Size:      int64(len(content)),
	}, nil
}

// WriteFileContent implements the WriteFileContent RPC method
func (s *FilesystemService) WriteFileContent(ctx context.Context, req *pb.WriteFileContentRequest) (*pb.OperationResponse, error) {
	var validPath string
	var err error

	// Handle both direct path and file handle
	if req.FileHandle != "" {
		fileEditor.mu.RLock()
		session, exists := fileEditor.sessions[req.FileHandle]
		fileEditor.mu.RUnlock()

		if !exists {
			return &pb.OperationResponse{
				Success: false,
				Error:   "Invalid file handle",
			}, nil
		}

		validPath = session.Path

		// Check if session allows writing
		if session.Mode == pb.FileOpenMode_READ_ONLY {
			return &pb.OperationResponse{
				Success: false,
				Error:   "File opened in read-only mode",
			}, nil
		}

		// Mark session as having changes
		fileEditor.mu.Lock()
		session.HasChanges = true
		session.LastAccess = time.Now()
		fileEditor.mu.Unlock()
	} else {
		validPath, err = s.validatePath(req.Path)
		if err != nil {
			return nil, err
		}
	}

	// Create backup if requested
	if req.CreateBackup {
		backupPath := validPath + ".backup." + strconv.FormatInt(time.Now().Unix(), 10)
		if _, err := os.Stat(validPath); err == nil {
			if err := s.copyFileForBackup(validPath, backupPath); err != nil {
				return &pb.OperationResponse{
					Success: false,
					Error:   fmt.Sprintf("Failed to create backup: %v", err),
				}, nil
			}
		}
	}

	// Write content to file
	flags := os.O_WRONLY | os.O_CREATE
	if req.Truncate {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		return &pb.OperationResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to open file for writing: %v", err),
		}, nil
	}
	defer file.Close()

	// Write content
	if _, err := file.WriteString(req.Content); err != nil {
		return &pb.OperationResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to write content: %v", err),
		}, nil
	}

	return &pb.OperationResponse{
		Success: true,
		Message: fmt.Sprintf("File content written successfully (%d bytes)", len(req.Content)),
	}, nil
}

// GetFileLines implements the GetFileLines RPC method
func (s *FilesystemService) GetFileLines(ctx context.Context, req *pb.GetFileLinesRequest) (*pb.FileLinesResponse, error) {
	var validPath string
	var err error

	// Handle both direct path and file handle
	if req.FileHandle != "" {
		fileEditor.mu.RLock()
		session, exists := fileEditor.sessions[req.FileHandle]
		fileEditor.mu.RUnlock()

		if !exists {
			return &pb.FileLinesResponse{
				Success: false,
				Error:   "Invalid file handle",
			}, nil
		}

		validPath = session.Path
	} else {
		validPath, err = s.validatePath(req.Path)
		if err != nil {
			return nil, err
		}
	}

	// Open file for reading
	file, err := os.Open(validPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &pb.FileLinesResponse{
				Success: false,
				Error:   "File does not exist",
			}, nil
		}
		return &pb.FileLinesResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to open file: %v", err),
		}, nil
	}
	defer file.Close()

	// Read lines
	scanner := bufio.NewScanner(file)
	var lines []*pb.FileLine
	lineNumber := int32(1)
	totalLines := int32(0)

	for scanner.Scan() {
		totalLines++

		// Check if line is within requested range
		if req.StartLine > 0 && lineNumber < req.StartLine {
			lineNumber++
			continue
		}

		if req.EndLine > 0 && lineNumber > req.EndLine {
			break
		}

		content := scanner.Text()

		fileLine := &pb.FileLine{
			Content: content,
			Length:  int32(len(content)),
		}

		if req.IncludeLineNumbers {
			fileLine.LineNumber = lineNumber
		}

		lines = append(lines, fileLine)
		lineNumber++
	}

	if err := scanner.Err(); err != nil {
		return &pb.FileLinesResponse{
			Success: false,
			Error:   fmt.Sprintf("Error reading file: %v", err),
		}, nil
	}

	return &pb.FileLinesResponse{
		Success:    true,
		Lines:      lines,
		TotalLines: totalLines,
	}, nil
}

// UpdateFileLines implements the UpdateFileLines RPC method
func (s *FilesystemService) UpdateFileLines(ctx context.Context, req *pb.UpdateFileLinesRequest) (*pb.OperationResponse, error) {
	var validPath string
	var err error

	// Handle both direct path and file handle
	if req.FileHandle != "" {
		fileEditor.mu.RLock()
		session, exists := fileEditor.sessions[req.FileHandle]
		fileEditor.mu.RUnlock()

		if !exists {
			return &pb.OperationResponse{
				Success: false,
				Error:   "Invalid file handle",
			}, nil
		}

		validPath = session.Path

		// Check if session allows writing
		if session.Mode == pb.FileOpenMode_READ_ONLY {
			return &pb.OperationResponse{
				Success: false,
				Error:   "File opened in read-only mode",
			}, nil
		}
	} else {
		validPath, err = s.validatePath(req.Path)
		if err != nil {
			return nil, err
		}
	}

	// Create backup if requested
	if req.CreateBackup {
		backupPath := validPath + ".backup." + strconv.FormatInt(time.Now().Unix(), 10)
		if err := s.copyFileForBackup(validPath, backupPath); err != nil {
			return &pb.OperationResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to create backup: %v", err),
			}, nil
		}
	}

	// Read current file content
	content, err := os.ReadFile(validPath)
	if err != nil {
		return &pb.OperationResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read file: %v", err),
		}, nil
	}

	// Split into lines
	lines := strings.Split(string(content), "\n")

	// Apply updates
	for _, update := range req.Updates {
		lineIdx := int(update.LineNumber) - 1 // Convert to 0-based index

		switch update.Operation {
		case pb.LineOperation_REPLACE:
			if lineIdx >= 0 && lineIdx < len(lines) {
				lines[lineIdx] = update.NewContent
			}
		case pb.LineOperation_INSERT_BEFORE:
			if lineIdx >= 0 && lineIdx <= len(lines) {
				lines = append(lines[:lineIdx], append([]string{update.NewContent}, lines[lineIdx:]...)...)
			}
		case pb.LineOperation_INSERT_AFTER:
			if lineIdx >= 0 && lineIdx < len(lines) {
				lines = append(lines[:lineIdx+1], append([]string{update.NewContent}, lines[lineIdx+1:]...)...)
			}
		case pb.LineOperation_DELETE:
			if lineIdx >= 0 && lineIdx < len(lines) {
				lines = append(lines[:lineIdx], lines[lineIdx+1:]...)
			}
		}
	}

	// Write updated content back to file
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(validPath, []byte(newContent), 0644); err != nil {
		return &pb.OperationResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to write updated content: %v", err),
		}, nil
	}

	// Mark session as having changes if using file handle
	if req.FileHandle != "" {
		fileEditor.mu.Lock()
		if session, exists := fileEditor.sessions[req.FileHandle]; exists {
			session.HasChanges = true
			session.LastAccess = time.Now()
		}
		fileEditor.mu.Unlock()
	}

	return &pb.OperationResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully applied %d line updates", len(req.Updates)),
	}, nil
}

// LockFile implements the LockFile RPC method
func (s *FilesystemService) LockFile(ctx context.Context, req *pb.LockFileRequest) (*pb.LockFileResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Clean up expired locks first
	fileEditor.cleanupExpiredLocks()

	fileEditor.mu.Lock()
	defer fileEditor.mu.Unlock()

	// Check if file is already locked
	if existingLock, exists := fileEditor.locks[validPath]; exists {
		if time.Now().Before(existingLock.ExpiresAt) {
			// Lock is still valid
			if req.LockType == pb.LockType_EXCLUSIVE || existingLock.Type == pb.LockType_EXCLUSIVE {
				return &pb.LockFileResponse{
					Success: false,
					Error:   "File is already locked",
				}, nil
			}
			// Allow shared locks
		} else {
			// Lock has expired, remove it
			delete(fileEditor.locks, validPath)
		}
	}

	// Create new lock
	lockID := generateLockID()
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Minute // Default 5 minutes
	}

	lock := &FileLock{
		LockID:    lockID,
		Path:      validPath,
		Type:      req.LockType,
		ExpiresAt: time.Now().Add(timeout),
		Owner:     "", // For future multi-user support
	}

	fileEditor.locks[validPath] = lock

	return &pb.LockFileResponse{
		Success:   true,
		LockId:    lockID,
		ExpiresAt: lock.ExpiresAt.Unix(),
	}, nil
}

// UnlockFile implements the UnlockFile RPC method
func (s *FilesystemService) UnlockFile(ctx context.Context, req *pb.UnlockFileRequest) (*pb.OperationResponse, error) {
	validPath, err := s.validatePath(req.Path)
	if err != nil {
		return nil, err
	}

	fileEditor.mu.Lock()
	defer fileEditor.mu.Unlock()

	lock, exists := fileEditor.locks[validPath]
	if !exists {
		return &pb.OperationResponse{
			Success: false,
			Error:   "No lock found for this file",
		}, nil
	}

	if lock.LockID != req.LockId {
		return &pb.OperationResponse{
			Success: false,
			Error:   "Invalid lock ID",
		}, nil
	}

	delete(fileEditor.locks, validPath)

	return &pb.OperationResponse{
		Success: true,
		Message: "File unlocked successfully",
	}, nil
}

// copyFileForBackup creates a backup copy of a file
func (s *FilesystemService) copyFileForBackup(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
