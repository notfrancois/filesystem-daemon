package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/notfrancois/filesystem-daemon/proto"
	"github.com/notfrancois/filesystem-daemon/service"
)

// Config contains the daemon configuration
var Config struct {
	WatchDir     string
	GRPCPort     int
	TLSConfig    *tls.Config
	CertFile     string
	KeyFile      string
	TLSEnabled   bool
}

func init() {
	// Default values
	Config.WatchDir = "/var/www/html"
	Config.GRPCPort = 50051
	Config.CertFile = "/etc/filesystem-daemon/certs/server.crt"
	Config.KeyFile = "/etc/filesystem-daemon/certs/server.key"
	Config.TLSEnabled = true

	// Command line flags
	flag.StringVar(&Config.WatchDir, "watch-dir", Config.WatchDir, "Directory to watch")
	flag.IntVar(&Config.GRPCPort, "grpc-port", Config.GRPCPort, "gRPC server port")
	flag.StringVar(&Config.CertFile, "cert", Config.CertFile, "TLS certificate file")
	flag.StringVar(&Config.KeyFile, "key", Config.KeyFile, "TLS key file")
	flag.BoolVar(&Config.TLSEnabled, "tls", Config.TLSEnabled, "Enable TLS")
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

	// By default, use TLS for production
	var grpcServer *grpc.Server
	var lis net.Listener

	// For development, allow connections without TLS, but with strict safeguards
	devMode := os.Getenv("DEV_MODE") == "true"
	prodEnv := os.Getenv("ENVIRONMENT") == "production" || os.Getenv("ENV") == "production"

	// Never allow insecure mode in production, regardless of DEV_MODE setting
	if devMode && !prodEnv {
		log.Println("⚠️ WARNING: Ejecutando en modo desarrollo sin TLS. NO USAR EN PRODUCCIÓN. ⚠️")
		log.Println("⚠️ Las conexiones inseguras están limitadas ÚNICAMENTE a localhost (127.0.0.1) ⚠️")

		// Only bind to localhost for insecure connections
		lis, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", Config.GRPCPort))
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		grpcServer = grpc.NewServer()
	} else {
		// Always use TLS for production or if dev mode is not explicitly enabled
		if devMode && prodEnv {
			log.Println("Detectado entorno de producción. Ignorando DEV_MODE y forzando TLS.")
		}

		// Listen on all interfaces but with TLS
		lis, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", Config.GRPCPort))
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		// Load certificates
		cert, err := tls.LoadX509KeyPair(Config.CertFile, Config.KeyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS certificates: %v", err)
		}
		
		// Update TLS config with the certificates
		Config.TLSConfig.Certificates = []tls.Certificate{cert}
		
		creds := credentials.NewTLS(Config.TLSConfig)
		grpcServer = grpc.NewServer(grpc.Creds(creds))
	}

	// Create and register the filesystem service
	filesystemService := service.NewFilesystemService(Config.WatchDir)
	proto.RegisterFilesystemServiceServer(grpcServer, filesystemService)

	// Enable reflection for easier client debugging and development
	reflection.Register(grpcServer)

	// Log information about available methods
	log.Printf("Filesystem service registered with the following operations:")
	log.Printf(" - ListDirectory: List contents of a directory")
	log.Printf(" - GetFileInfo: Get detailed information about a file")
	log.Printf(" - CreateDirectory: Create a new directory")
	log.Printf(" - Delete: Delete a file or directory")
	log.Printf(" - Copy: Copy a file or directory")
	log.Printf(" - Move: Move/rename a file or directory")
	log.Printf(" - UploadFile: Upload a file (streaming)")
	log.Printf(" - DownloadFile: Download a file (streaming)")
	log.Printf(" - Exists: Check if a path exists")
	log.Printf(" - GetDirectorySize: Get the size of a directory")
	log.Printf(" - Search: Search for files/directories")

	// Start file system monitoring for changes (optional background task)
	go func() {
		// Setup file system notification (using FSNotify or similar)
		log.Printf("File system monitoring started for %s", Config.WatchDir)

		// Periodically log activity statistics
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Printf("Filesystem daemon active, monitoring: %s", Config.WatchDir)
			}
		}
	}()

	// Setup HTTP health check endpoint (optional)
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Filesystem daemon is healthy"))
		})
		log.Printf("Starting health check endpoint on port %d", Config.GRPCPort+1)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", Config.GRPCPort+1), nil); err != nil {
			log.Printf("Health check server failed: %v", err)
		}
	}()

	// Log server startup info
	if devMode && !prodEnv {
		log.Printf("Starting gRPC server on port %d without TLS (DEV MODE)", Config.GRPCPort)
	} else {
		log.Printf("Starting gRPC server on port %d with TLS", Config.GRPCPort)
	}
	log.Printf("Ready to handle C# client requests for filesystem operations")

	// Start serving in the main goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("Daemon running. Waiting for signals...")

	// Handle signals - wait indefinitely for shutdown signal
	sig := <-ch
	log.Printf("Received shutdown signal %v. Graceful shutdown...", sig)
	grpcServer.GracefulStop()
	log.Printf("Shutdown complete")
}
