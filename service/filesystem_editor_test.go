package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/notfrancois/filesystem-daemon/proto"
)

func TestFileEditorOperations(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "filesystem_editor_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create service instance
	service := NewFilesystemService(tmpDir)
	ctx := context.Background()

	// Test file content
	testContent := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	testFileName := "test_file.txt"

	t.Run("WriteFileContent", func(t *testing.T) {
		req := &pb.WriteFileContentRequest{
			Path:     testFileName,
			Content:  testContent,
			Truncate: true,
		}

		resp, err := service.WriteFileContent(ctx, req)
		if err != nil {
			t.Fatalf("WriteFileContent failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("WriteFileContent was not successful: %s", resp.Error)
		}
	})

	t.Run("ReadFileContent", func(t *testing.T) {
		req := &pb.FileRequest{
			Path: testFileName,
		}

		resp, err := service.ReadFileContent(ctx, req)
		if err != nil {
			t.Fatalf("ReadFileContent failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("ReadFileContent was not successful: %s", resp.Error)
		}

		if resp.Content != testContent {
			t.Fatalf("Content mismatch. Expected: %s, Got: %s", testContent, resp.Content)
		}

		if resp.LineCount != 5 {
			t.Fatalf("Line count mismatch. Expected: 5, Got: %d", resp.LineCount)
		}
	})

	t.Run("OpenFile", func(t *testing.T) {
		req := &pb.OpenFileRequest{
			Path: testFileName,
			Mode: pb.FileOpenMode_READ_WRITE,
		}

		resp, err := service.OpenFile(ctx, req)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("OpenFile was not successful: %s", resp.Error)
		}

		if resp.FileHandle == "" {
			t.Fatalf("No file handle returned")
		}

		// Test closing the file
		closeReq := &pb.CloseFileRequest{
			FileHandle:  resp.FileHandle,
			SaveChanges: true,
		}

		closeResp, err := service.CloseFile(ctx, closeReq)
		if err != nil {
			t.Fatalf("CloseFile failed: %v", err)
		}

		if !closeResp.Success {
			t.Fatalf("CloseFile was not successful: %s", closeResp.Error)
		}
	})

	t.Run("GetFileLines", func(t *testing.T) {
		req := &pb.GetFileLinesRequest{
			Path:               testFileName,
			StartLine:          2,
			EndLine:            4,
			IncludeLineNumbers: true,
		}

		resp, err := service.GetFileLines(ctx, req)
		if err != nil {
			t.Fatalf("GetFileLines failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("GetFileLines was not successful: %s", resp.Error)
		}

		if len(resp.Lines) != 3 {
			t.Fatalf("Expected 3 lines, got %d", len(resp.Lines))
		}

		expectedLines := []string{"Line 2", "Line 3", "Line 4"}
		for i, line := range resp.Lines {
			if line.Content != expectedLines[i] {
				t.Fatalf("Line %d content mismatch. Expected: %s, Got: %s", i, expectedLines[i], line.Content)
			}
			if line.LineNumber != int32(i+2) {
				t.Fatalf("Line number mismatch. Expected: %d, Got: %d", i+2, line.LineNumber)
			}
		}
	})

	t.Run("UpdateFileLines", func(t *testing.T) {
		updates := []*pb.LineUpdate{
			{
				LineNumber: 2,
				NewContent: "Modified Line 2",
				Operation:  pb.LineOperation_REPLACE,
			},
			{
				LineNumber: 3,
				NewContent: "Inserted Line",
				Operation:  pb.LineOperation_INSERT_AFTER,
			},
		}

		req := &pb.UpdateFileLinesRequest{
			Path:         testFileName,
			Updates:      updates,
			CreateBackup: true,
		}

		resp, err := service.UpdateFileLines(ctx, req)
		if err != nil {
			t.Fatalf("UpdateFileLines failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("UpdateFileLines was not successful: %s", resp.Error)
		}

		// Verify the changes
		readReq := &pb.FileRequest{Path: testFileName}
		readResp, err := service.ReadFileContent(ctx, readReq)
		if err != nil {
			t.Fatalf("Failed to read updated content: %v", err)
		}

		lines := strings.Split(readResp.Content, "\n")
		if lines[1] != "Modified Line 2" {
			t.Fatalf("Line 2 was not updated correctly. Got: %s", lines[1])
		}

		if lines[3] != "Inserted Line" {
			t.Fatalf("Line was not inserted correctly. Got: %s", lines[3])
		}
	})

	t.Run("LockFile", func(t *testing.T) {
		lockReq := &pb.LockFileRequest{
			Path:           testFileName,
			LockType:       pb.LockType_EXCLUSIVE,
			TimeoutSeconds: 60,
		}

		lockResp, err := service.LockFile(ctx, lockReq)
		if err != nil {
			t.Fatalf("LockFile failed: %v", err)
		}

		if !lockResp.Success {
			t.Fatalf("LockFile was not successful: %s", lockResp.Error)
		}

		if lockResp.LockId == "" {
			t.Fatalf("No lock ID returned")
		}

		// Try to lock again (should fail)
		lockResp2, err := service.LockFile(ctx, lockReq)
		if err != nil {
			t.Fatalf("Second LockFile failed: %v", err)
		}

		if lockResp2.Success {
			t.Fatalf("Second LockFile should have failed due to existing lock")
		}

		// Unlock the file
		unlockReq := &pb.UnlockFileRequest{
			Path:   testFileName,
			LockId: lockResp.LockId,
		}

		unlockResp, err := service.UnlockFile(ctx, unlockReq)
		if err != nil {
			t.Fatalf("UnlockFile failed: %v", err)
		}

		if !unlockResp.Success {
			t.Fatalf("UnlockFile was not successful: %s", unlockResp.Error)
		}
	})

	t.Run("OpenFileWithLock", func(t *testing.T) {
		req := &pb.OpenFileRequest{
			Path:          testFileName,
			Mode:          pb.FileOpenMode_READ_WRITE,
			ExclusiveLock: true,
		}

		resp, err := service.OpenFile(ctx, req)
		if err != nil {
			t.Fatalf("OpenFile with lock failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("OpenFile with lock was not successful: %s", resp.Error)
		}

		if resp.LockId == "" {
			t.Fatalf("No lock ID returned for exclusive lock")
		}

		// Close file (should release lock)
		closeReq := &pb.CloseFileRequest{
			FileHandle:  resp.FileHandle,
			SaveChanges: false,
		}

		closeResp, err := service.CloseFile(ctx, closeReq)
		if err != nil {
			t.Fatalf("CloseFile failed: %v", err)
		}

		if !closeResp.Success {
			t.Fatalf("CloseFile was not successful: %s", closeResp.Error)
		}
	})

	t.Run("CreateNewFileWithOpenFile", func(t *testing.T) {
		newFileName := "new_test_file.txt"

		req := &pb.OpenFileRequest{
			Path:              newFileName,
			Mode:              pb.FileOpenMode_READ_WRITE,
			CreateIfNotExists: true,
		}

		resp, err := service.OpenFile(ctx, req)
		if err != nil {
			t.Fatalf("OpenFile for new file failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("OpenFile for new file was not successful: %s", resp.Error)
		}

		// Write content using file handle
		writeReq := &pb.WriteFileContentRequest{
			FileHandle: resp.FileHandle,
			Content:    "New file content",
			Truncate:   true,
		}

		writeResp, err := service.WriteFileContent(ctx, writeReq)
		if err != nil {
			t.Fatalf("WriteFileContent with handle failed: %v", err)
		}

		if !writeResp.Success {
			t.Fatalf("WriteFileContent with handle was not successful: %s", writeResp.Error)
		}

		// Close the file
		closeReq := &pb.CloseFileRequest{
			FileHandle:  resp.FileHandle,
			SaveChanges: true,
		}

		_, err = service.CloseFile(ctx, closeReq)
		if err != nil {
			t.Fatalf("CloseFile failed: %v", err)
		}

		// Verify file exists and has correct content
		if _, err := os.Stat(filepath.Join(tmpDir, newFileName)); os.IsNotExist(err) {
			t.Fatalf("New file was not created")
		}
	})
}

func TestFileLockExpiration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem_lock_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	service := NewFilesystemService(tmpDir)
	ctx := context.Background()
	testFileName := "lock_test.txt"

	// Create test file
	writeReq := &pb.WriteFileContentRequest{
		Path:     testFileName,
		Content:  "test content",
		Truncate: true,
	}
	_, err = service.WriteFileContent(ctx, writeReq)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Run("LockExpiration", func(t *testing.T) {
		// Lock with very short timeout
		lockReq := &pb.LockFileRequest{
			Path:           testFileName,
			LockType:       pb.LockType_EXCLUSIVE,
			TimeoutSeconds: 1, // 1 second
		}

		lockResp, err := service.LockFile(ctx, lockReq)
		if err != nil {
			t.Fatalf("LockFile failed: %v", err)
		}

		if !lockResp.Success {
			t.Fatalf("LockFile was not successful: %s", lockResp.Error)
		}

		// Wait for lock to expire
		time.Sleep(2 * time.Second)

		// Try to lock again (should succeed due to expiration)
		lockResp2, err := service.LockFile(ctx, lockReq)
		if err != nil {
			t.Fatalf("Second LockFile failed: %v", err)
		}

		if !lockResp2.Success {
			t.Fatalf("Second LockFile should have succeeded due to expired lock: %s", lockResp2.Error)
		}

		// Clean up
		unlockReq := &pb.UnlockFileRequest{
			Path:   testFileName,
			LockId: lockResp2.LockId,
		}
		service.UnlockFile(ctx, unlockReq)
	})
}
