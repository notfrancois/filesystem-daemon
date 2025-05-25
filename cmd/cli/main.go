package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/notfrancois/filesystem-daemon/proto"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// CLI configuration
var (
	serverAddress string
	useTLS        bool
	certFile      string
	timeout       int
	outputFormat  string
	verbose       bool
)

// Client connection
var (
	conn   *grpc.ClientConn
	client proto.FilesystemServiceClient
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "fsdaemon",
		Short: "Filesystem Daemon CLI client",
		Long: `Command line interface for the Filesystem Daemon service.
Allows operations on files and directories through a remote daemon.`,
		PersistentPreRun: connectToDaemon,
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if conn != nil {
				conn.Close()
			}
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverAddress, "server", "s", "localhost:50051", "Server address (host:port)")
	rootCmd.PersistentFlags().BoolVar(&useTLS, "tls", true, "Use TLS for connection")
	rootCmd.PersistentFlags().StringVar(&certFile, "cert", "", "TLS certificate file (for self-signed certs)")
	rootCmd.PersistentFlags().IntVarP(&timeout, "timeout", "t", 30, "Command timeout in seconds")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	// Add commands
	rootCmd.AddCommand(
		newListCommand(),
		newInfoCommand(),
		newExistsCommand(),
		newCreateDirCommand(),
		newDeleteCommand(),
		newCopyCommand(),
		newMoveCommand(),
		newUploadCommand(),
		newDownloadCommand(),
		newSearchCommand(),
		newHierarchyCommand(),
		newDirSizeCommand(),
		newStatusCommand(),
	)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// Connect to the daemon server
func connectToDaemon(cmd *cobra.Command, args []string) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Setup connection options
	var opts []grpc.DialOption
	if useTLS {
		var creds credentials.TransportCredentials
		if certFile != "" {
			// Use custom certificate
			creds, err = loadTLSCredentials(certFile)
			if err != nil {
				fmt.Printf("Failed to load TLS credentials: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Use system certificates
			creds = credentials.NewTLS(&tls.Config{})
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Connect to the server
	conn, err = grpc.DialContext(ctx, serverAddress, opts...)
	if err != nil {
		fmt.Printf("Failed to connect to server: %v\n", err)
		os.Exit(1)
	}

	// Create client
	client = proto.NewFilesystemServiceClient(conn)
}

// Load TLS credentials from file
func loadTLSCredentials(certFile string) (credentials.TransportCredentials, error) {
	// Load certificate file
	_, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}

	// Create credentials
	config := &tls.Config{
		InsecureSkipVerify: true, // Not recommended for production
	}

	return credentials.NewTLS(config), nil
}

// formatOutput formats the result based on the specified output format
func formatOutput(data interface{}) {
	switch outputFormat {
	case "json":
		jsonBytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Printf("Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println(string(jsonBytes))
	case "text":
		// Default text format handled by individual commands
		switch v := data.(type) {
		case string:
			fmt.Println(v)
		default:
			// For structs, try to print them in a readable way
			fmt.Printf("%+v\n", v)
		}
	default:
		fmt.Printf("%+v\n", data)
	}
}

// Create a new command for listing directory contents
func newListCommand() *cobra.Command {
	var recursive bool
	var pattern string

	cmd := &cobra.Command{
		Use:     "list [path]",
		Aliases: []string{"ls"},
		Short:   "List directory contents",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			// Make request
			request := &proto.ListRequest{
				Path:      path,
				Recursive: recursive,
				Pattern:   pattern,
			}

			response, err := client.ListDirectory(ctx, request)
			if err != nil {
				fmt.Printf("Error listing directory: %v\n", err)
				os.Exit(1)
			}

			// Process response
			if outputFormat == "json" {
				formatOutput(response)
			} else {
				// Display as text table
				fmt.Printf("Contents of %s:\n", path)
				fmt.Println("Type\tSize\tModified\t\tName")
				fmt.Println("--------------------------------------------------------------")
				for _, item := range response.Items {
					fileType := "F"
					if item.IsDirectory {
						fileType = "D"
					}
					modTime := time.Unix(item.ModifiedTime, 0).Format("2006-01-02 15:04:05")
					fmt.Printf("%s\t%d\t%s\t%s\n", fileType, item.Size, modTime, item.Name)
				}
				fmt.Printf("\nTotal: %d items\n", len(response.Items))
			}
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "List recursively")
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "", "Filter by pattern (e.g. *.go)")

	return cmd
}

// Create a new command for getting file info
func newInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info [path]",
		Aliases: []string{"stat"},
		Short:   "Get detailed file information",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.FileRequest{Path: args[0]}
			response, err := client.GetFileInfo(ctx, request)
			if err != nil {
				fmt.Printf("Error getting file info: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				fmt.Println("File Information:")
				fmt.Printf("Name:         %s\n", response.Name)
				fmt.Printf("Path:         %s\n", response.Path)
				fmt.Printf("Type:         %s\n", getTypeString(response.IsDirectory))
				fmt.Printf("Size:         %d bytes\n", response.Size)
				fmt.Printf("Modified:     %s\n", time.Unix(response.ModifiedTime, 0).Format(time.RFC1123))
				fmt.Printf("Created:      %s\n", time.Unix(response.CreationTime, 0).Format(time.RFC1123))
				fmt.Printf("Accessed:     %s\n", time.Unix(response.AccessTime, 0).Format(time.RFC1123))
				fmt.Printf("MIME Type:    %s\n", response.MimeType)
				fmt.Printf("Permissions:  %s\n", response.Permissions)
				fmt.Printf("Owner:        %s\n", response.Owner)
				fmt.Printf("Group:        %s\n", response.Group)
			}
		},
	}

	return cmd
}

// Create a new command for checking if a path exists
func newExistsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exists [path]",
		Short: "Check if a path exists",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.PathRequest{Path: args[0]}
			response, err := client.Exists(ctx, request)
			if err != nil {
				fmt.Printf("Error checking path: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Exists {
					pathType := "file"
					if response.IsDirectory {
						pathType = "directory"
					}
					fmt.Printf("Path exists and is a %s\n", pathType)
				} else {
					fmt.Println("Path does not exist")
				}
			}
		},
	}

	return cmd
}

// Create a new command for creating a directory
func newCreateDirCommand() *cobra.Command {
	var permissions int

	cmd := &cobra.Command{
		Use:     "mkdir [path]",
		Aliases: []string{"create-dir"},
		Short:   "Create a new directory",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.CreateDirectoryRequest{
				Path:        args[0],
				Permissions: int32(permissions),
			}

			response, err := client.CreateDirectory(ctx, request)
			if err != nil {
				fmt.Printf("Error creating directory: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Success {
					fmt.Printf("Directory created successfully: %s\n", args[0])
				} else {
					fmt.Printf("Failed to create directory: %s\n", response.Error)
				}
			}
		},
	}

	cmd.Flags().IntVarP(&permissions, "permissions", "m", 0755, "Directory permissions (octal)")

	return cmd
}

// Create a new command for deleting a file or directory
func newDeleteCommand() *cobra.Command {
	var recursive bool

	cmd := &cobra.Command{
		Use:     "delete [path]",
		Aliases: []string{"rm", "remove"},
		Short:   "Delete a file or directory",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.DeleteRequest{
				Path:      args[0],
				Recursive: recursive,
			}

			response, err := client.Delete(ctx, request)
			if err != nil {
				fmt.Printf("Error deleting path: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Success {
					fmt.Printf("Successfully deleted: %s\n", args[0])
				} else {
					fmt.Printf("Failed to delete: %s\n", response.Error)
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Delete directories recursively")

	return cmd
}

// Create a new command for copying a file or directory
func newCopyCommand() *cobra.Command {
	var overwrite bool

	cmd := &cobra.Command{
		Use:     "copy [source] [destination]",
		Aliases: []string{"cp"},
		Short:   "Copy a file or directory",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.CopyRequest{
				Source:      args[0],
				Destination: args[1],
				Overwrite:   overwrite,
			}

			response, err := client.Copy(ctx, request)
			if err != nil {
				fmt.Printf("Error copying: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Success {
					fmt.Printf("Successfully copied: %s -> %s\n", args[0], args[1])
				} else {
					fmt.Printf("Failed to copy: %s\n", response.Error)
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&overwrite, "overwrite", "f", false, "Overwrite destination if it exists")

	return cmd
}

// Create a new command for moving/renaming a file or directory
func newMoveCommand() *cobra.Command {
	var overwrite bool

	cmd := &cobra.Command{
		Use:     "move [source] [destination]",
		Aliases: []string{"mv", "rename"},
		Short:   "Move or rename a file or directory",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.MoveRequest{
				Source:      args[0],
				Destination: args[1],
				Overwrite:   overwrite,
			}

			response, err := client.Move(ctx, request)
			if err != nil {
				fmt.Printf("Error moving: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Success {
					fmt.Printf("Successfully moved: %s -> %s\n", args[0], args[1])
				} else {
					fmt.Printf("Failed to move: %s\n", response.Error)
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&overwrite, "overwrite", "f", false, "Overwrite destination if it exists")

	return cmd
}

// Create a new command for uploading a file
func newUploadCommand() *cobra.Command {
	var chunkSize int

	cmd := &cobra.Command{
		Use:   "upload [local_file] [remote_path]",
		Short: "Upload a file to the server",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			localFile := args[0]
			remotePath := args[1]

			// Open the local file
			file, err := os.Open(localFile)
			if err != nil {
				fmt.Printf("Error opening local file: %v\n", err)
				os.Exit(1)
			}
			defer file.Close()

			// Create upload stream
			stream, err := client.UploadFile(ctx)
			if err != nil {
				fmt.Printf("Error creating upload stream: %v\n", err)
				os.Exit(1)
			}

			// Get file info for progress reporting
			fileInfo, err := file.Stat()
			if err != nil {
				fmt.Printf("Error getting file info: %v\n", err)
				os.Exit(1)
			}
			totalSize := fileInfo.Size()

			// Read and send file in chunks
			buffer := make([]byte, chunkSize)
			totalSent := int64(0)
			for {
				n, err := file.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					fmt.Printf("Error reading file: %v\n", err)
					os.Exit(1)
				}

				// Send chunk
				chunk := &proto.FileChunk{
					FilePath: remotePath,
					Content:  buffer[:n],
					Offset:   totalSent,
					IsLast:   false,
				}
				
				if err := stream.Send(chunk); err != nil {
					fmt.Printf("Error sending chunk: %v\n", err)
					os.Exit(1)
				}

				totalSent += int64(n)
				
				// Print progress
				if verbose {
					progress := float64(totalSent) / float64(totalSize) * 100
					fmt.Printf("\rUploading: %.2f%% (%d/%d bytes)", progress, totalSent, totalSize)
				}
			}

			// Send last empty chunk to indicate end of file
			lastChunk := &proto.FileChunk{
				FilePath: remotePath,
				Content:  []byte{},
				Offset:   totalSent,
				IsLast:   true,
			}
			
			if err := stream.Send(lastChunk); err != nil {
				fmt.Printf("\nError sending final chunk: %v\n", err)
				os.Exit(1)
			}

			// Get response
			response, err := stream.CloseAndRecv()
			if err != nil {
				fmt.Printf("\nError receiving response: %v\n", err)
				os.Exit(1)
			}

			if verbose {
				fmt.Println()
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				if response.Success {
					fmt.Printf("Successfully uploaded %s to %s (%d bytes)\n", localFile, remotePath, totalSent)
				} else {
					fmt.Printf("Failed to upload file: %s\n", response.Error)
				}
			}
		},
	}

	cmd.Flags().IntVarP(&chunkSize, "chunk-size", "c", 1024*1024, "Chunk size in bytes")

	return cmd
}

// Create a new command for downloading a file
func newDownloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download [remote_path] [local_file]",
		Short: "Download a file from the server",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			remotePath := args[0]
			localFile := args[1]

			// Create local file
			file, err := os.Create(localFile)
			if err != nil {
				fmt.Printf("Error creating local file: %v\n", err)
				os.Exit(1)
			}
			defer file.Close()

			// Create download stream
			stream, err := client.DownloadFile(ctx, &proto.FileRequest{Path: remotePath})
			if err != nil {
				fmt.Printf("Error creating download stream: %v\n", err)
				os.Exit(1)
			}

			// Receive and write chunks
			totalReceived := int64(0)
			for {
				chunk, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					fmt.Printf("Error receiving chunk: %v\n", err)
					os.Exit(1)
				}

				// Write chunk to file
				n, err := file.Write(chunk.Content)
				if err != nil {
					fmt.Printf("Error writing to file: %v\n", err)
					os.Exit(1)
				}

				totalReceived += int64(n)
				
				// Print progress
				if verbose {
					fmt.Printf("\rDownloading: %d bytes received", totalReceived)
				}

				if chunk.IsLast {
					break
				}
			}

			if verbose {
				fmt.Println()
			}

			if outputFormat == "json" {
				result := map[string]interface{}{
					"success": true,
					"bytes_received": totalReceived,
					"local_file": localFile,
					"remote_path": remotePath,
				}
				formatOutput(result)
			} else {
				fmt.Printf("Successfully downloaded %s to %s (%d bytes)\n", remotePath, localFile, totalReceived)
			}
		},
	}

	return cmd
}

// Create a new command for searching files
func newSearchCommand() *cobra.Command {
	var (
		caseSensitive  bool
		recursive      bool
		directoriesOnly bool
		filesOnly      bool
		maxResults     int
	)

	cmd := &cobra.Command{
		Use:   "search [path] [pattern]",
		Short: "Search for files and directories",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.SearchRequest{
				BasePath:        args[0],
				Pattern:         args[1],
				CaseSensitive:   caseSensitive,
				Recursive:       recursive,
				DirectoriesOnly: directoriesOnly,
				FilesOnly:       filesOnly,
				MaxResults:      int32(maxResults),
			}

			response, err := client.Search(ctx, request)
			if err != nil {
				fmt.Printf("Error searching: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				fmt.Printf("Search results for pattern '%s' in '%s':\n", args[1], args[0])
				fmt.Println("Type\tSize\tModified\t\tPath")
				fmt.Println("--------------------------------------------------------------")
				for _, item := range response.Items {
					fileType := "F"
					if item.IsDirectory {
						fileType = "D"
					}
					modTime := time.Unix(item.ModifiedTime, 0).Format("2006-01-02 15:04:05")
					fmt.Printf("%s\t%d\t%s\t%s\n", fileType, item.Size, modTime, item.Path)
				}
				fmt.Printf("\nTotal matches: %d\n", len(response.Items))
			}
		},
	}

	cmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "Use case-sensitive matching")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Search recursively")
	cmd.Flags().BoolVarP(&directoriesOnly, "dirs-only", "d", false, "Match directories only")
	cmd.Flags().BoolVarP(&filesOnly, "files-only", "f", false, "Match files only")
	cmd.Flags().IntVarP(&maxResults, "max-results", "m", 100, "Maximum number of results")

	return cmd
}

// Create a new command for getting directory hierarchy
func newHierarchyCommand() *cobra.Command {
	var (
		maxDepth int
		pattern  string
	)

	cmd := &cobra.Command{
		Use:     "hierarchy [path]",
		Aliases: []string{"tree"},
		Short:   "Display directory hierarchy",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			request := &proto.HierarchyRequest{
				Path:     path,
				MaxDepth: int32(maxDepth),
				Pattern:  pattern,
			}

			response, err := client.GetHierarchy(ctx, request)
			if err != nil {
				fmt.Printf("Error getting hierarchy: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				fmt.Printf("Hierarchy for %s:\n", path)
				printHierarchy(response.Root, "", true)
				
				if response.Truncated {
					fmt.Println("\nNote: Hierarchy was truncated due to max depth limit.")
				}
			}
		},
	}

	cmd.Flags().IntVarP(&maxDepth, "max-depth", "d", 0, "Maximum depth (0 for unlimited)")
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "", "Filter by pattern")

	return cmd
}

// Create a new command for getting directory size
func newDirSizeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "size [path]",
		Aliases: []string{"du"},
		Short:   "Get the size of a directory",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			request := &proto.PathRequest{Path: args[0]}
			response, err := client.GetDirectorySize(ctx, request)
			if err != nil {
				fmt.Printf("Error getting directory size: %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				formatOutput(response)
			} else {
				fmt.Printf("Size of %s: %s (%d bytes)\n", args[0], formatSize(response.Size), response.Size)
			}
		},
	}

	return cmd
}

// Create a new command for checking daemon status
func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			// We use Exists on the root dir as a simple ping
			request := &proto.PathRequest{Path: "/"}
			
			startTime := time.Now()
			_, err := client.Exists(ctx, request)
			latency := time.Since(startTime)
			
			if err != nil {
				fmt.Printf("Daemon status: ERROR - %v\n", err)
				os.Exit(1)
			}

			if outputFormat == "json" {
				status := map[string]interface{}{
					"status":  "running",
					"latency": latency.String(),
					"latency_ms": latency.Milliseconds(),
					"address": serverAddress,
					"tls":     useTLS,
				}
				formatOutput(status)
			} else {
				fmt.Println("Daemon Status:")
				fmt.Printf("Status:   Running\n")
				fmt.Printf("Address:  %s\n", serverAddress)
				fmt.Printf("TLS:      %v\n", useTLS)
				fmt.Printf("Latency:  %s\n", latency)
			}
		},
	}

	return cmd
}

// Utility functions

// Get string representation of file type
func getTypeString(isDirectory bool) string {
	if isDirectory {
		return "Directory"
	}
	return "File"
}

// Format file size to human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

// Print hierarchy recursively
func printHierarchy(item *proto.FileItem, prefix string, isLast bool) {
	// Print current item
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	// Print item with the right marker
	fmt.Printf("%s%s%s\n", prefix, marker, item.Name)

	// Prepare the prefix for children
	childPrefix := prefix + "│   "
	if isLast {
		childPrefix = prefix + "    "
	}

	// Print children if any
	for i, child := range item.Children {
		isLastChild := i == len(item.Children)-1
		printHierarchy(child, childPrefix, isLastChild)
	}
}
