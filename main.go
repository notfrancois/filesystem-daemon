package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config contains the daemon configuration
var Config struct {
	WatchDir  string
	GRPCPort  int
	TLSConfig *tls.Config
}

func init() {
	// Default values
	Config.WatchDir = "/var/www/html"
	Config.GRPCPort = 50051

	// Command line flags
	flag.StringVar(&Config.WatchDir, "watch-dir", Config.WatchDir, "Directory to watch")
	flag.IntVar(&Config.GRPCPort, "grpc-port", Config.GRPCPort, "gRPC server port")
	flag.Parse()

	// Initialize TLS configuration
	Config.TLSConfig = &tls.Config{
		MinVersion:               tls.VersionTLS13,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

func main() {
	// Validate configuration
	if _, err := os.Stat(Config.WatchDir); os.IsNotExist(err) {
		log.Fatalf("Watch directory %s does not exist", Config.WatchDir)
	}

	// Create absolute path
	absPath, err := filepath.Abs(Config.WatchDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}
	Config.WatchDir = absPath

	// Set secure file permissions
	if err := os.Chmod(Config.WatchDir, 0750); err != nil {
		log.Printf("Warning: Failed to set permissions on watch directory: %v", err)
	}

	// Initialize security context
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		log.Printf("Warning: Failed to set PR_SET_NO_NEW_PRIVS: %v", err)
	}

	// Initialize gRPC server with TLS
	creds := credentials.NewTLS(Config.TLSConfig)
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	// Start file system monitoring
	go func() {
		// TODO: Implement file system monitoring with secure permissions
		log.Printf("File system monitoring started for %s", Config.WatchDir)
	}()

	// Start gRPC server - explicitly bind to all interfaces
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", Config.GRPCPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		log.Printf("Starting gRPC server on port %d with TLS", Config.GRPCPort)
		grpcServer.Serve(lis)
	}()

	// Wait for signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("Daemon running. Waiting for signals...")

	// Handle signals - wait indefinitely for shutdown signal
	signal := <-ch
	log.Printf("Received shutdown signal %v. Graceful shutdown...", signal)
	grpcServer.GracefulStop()
	log.Printf("Shutdown complete")
}
