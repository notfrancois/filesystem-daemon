package service

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
)

// AssetValidator handles validation of uploaded files
type AssetValidator struct {
	MaxFileSize  int64
	AllowedExts  map[string]bool
	AllowedMimes map[string]bool
}

// NewAssetValidator creates a new asset validator with the given constraints
func NewAssetValidator(maxSize int64, extensions []string) *AssetValidator {
	allowedExts := make(map[string]bool)
	allowedMimes := make(map[string]bool)

	for _, ext := range extensions {
		ext = strings.ToLower(strings.TrimPrefix(ext, "."))
		allowedExts[ext] = true

		// Map extension to MIME type
		mimeType := mime.TypeByExtension("." + ext)
		if mimeType != "" {
			allowedMimes[mimeType] = true
		}
	}

	// Add common web asset MIME types
	commonMimes := map[string]bool{
		"text/html":              true,
		"text/css":               true,
		"text/javascript":        true,
		"application/javascript": true,
		"application/json":       true,
		"image/jpeg":             true,
		"image/png":              true,
		"image/gif":              true,
		"image/svg+xml":          true,
		"image/webp":             true,
		"application/pdf":        true,
		"font/woff":              true,
		"font/woff2":             true,
		"application/font-woff":  true,
		"application/font-woff2": true,
	}

	// Merge with detected MIME types
	for mime := range commonMimes {
		allowedMimes[mime] = true
	}

	return &AssetValidator{
		MaxFileSize:  maxSize,
		AllowedExts:  allowedExts,
		AllowedMimes: allowedMimes,
	}
}

// ValidateFile checks if a file meets the validation criteria
func (v *AssetValidator) ValidateFile(path string, size int64) error {
	// Check file size
	if size > v.MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum allowed size %d bytes", size, v.MaxFileSize)
	}

	// Check extension
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if len(ext) == 0 {
		return fmt.Errorf("file has no extension")
	}

	if !v.AllowedExts[ext] {
		return fmt.Errorf("file extension '%s' is not allowed", ext)
	}

	return nil
}

// ValidateFileName checks if a filename is valid for web assets
func (v *AssetValidator) ValidateFileName(filename string) error {
	// Check for dangerous characters
	if strings.ContainsAny(filename, "<>:\"|?*") {
		return fmt.Errorf("filename contains invalid characters")
	}

	// Check for relative path attempts
	if strings.Contains(filename, "..") {
		return fmt.Errorf("filename cannot contain '..'")
	}

	// Check for leading/trailing spaces
	if strings.TrimSpace(filename) != filename {
		return fmt.Errorf("filename cannot have leading or trailing spaces")
	}

	return nil
}

// GetAllowedExtensions returns the list of allowed extensions
func (v *AssetValidator) GetAllowedExtensions() []string {
	var exts []string
	for ext := range v.AllowedExts {
		exts = append(exts, ext)
	}
	return exts
}
