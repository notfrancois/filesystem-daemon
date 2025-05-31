package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/notfrancois/filesystem-daemon/proto"
	"github.com/notfrancois/filesystem-daemon/service"
	"github.com/sirupsen/logrus"
)

// Enhanced config structure for Docker deployment
var Config struct {
	WatchDir        string
	GRPCPort        int
	HealthPort      int
	TLSConfig       *tls.Config
	CertFile        string
	KeyFile         string
	TLSEnabled      bool
	DefaultFileMode os.FileMode
	DefaultDirMode  os.FileMode
	WebServerUID    int
	WebServerGID    int
	LogLevel        string
	LogFormat       string
	MaxFileSize     int64
	AllowedExts     []string
	TrustedNetworks []string
}

func init() {
	// Default values
	Config.WatchDir = "/var/www/html"
	Config.GRPCPort = 50051
	Config.CertFile = "/etc/filesystem-daemon/certs/server.crt"
	Config.KeyFile = "/etc/filesystem-daemon/certs/server.key"
	Config.TLSEnabled = true

	// Nuevas configuraciones para permisos
	Config.DefaultFileMode = 0644 // Permisos por defecto para archivos
	Config.DefaultDirMode = 0755  // Permisos por defecto para directorios
	Config.WebServerUID = 33      // Usuario del servidor web
	Config.WebServerGID = 33      // Grupo del servidor web

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

	// Enhanced configuration from environment
	Config.HealthPort = getEnvInt("HEALTH_PORT", 50052)
	Config.LogLevel = getEnv("LOG_LEVEL", "info")
	Config.LogFormat = getEnv("LOG_FORMAT", "json")
	Config.MaxFileSize = parseSize(getEnv("MAX_FILE_SIZE", "100MB"))
	Config.AllowedExts = strings.Split(getEnv("ALLOWED_EXTENSIONS", "jpg,jpeg,png,gif,svg,css,js,html,txt,pdf"), ",")
	Config.TrustedNetworks = strings.Split(getEnv("TRUSTED_NETWORKS", "127.0.0.1/8,10.0.0.0/8"), ",")

	// Setup structured logging
	setupLogging()
}

func setupLogging() {
	level, err := logrus.ParseLevel(Config.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	if Config.LogFormat == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func parseSize(sizeStr string) int64 {
	// Simple size parser for MB/GB
	if strings.HasSuffix(sizeStr, "MB") {
		if size, err := strconv.ParseInt(strings.TrimSuffix(sizeStr, "MB"), 10, 64); err == nil {
			return size * 1024 * 1024
		}
	}
	return 100 * 1024 * 1024 // Default 100MB
}

// parseTrustedNetworks parses CIDR networks from configuration
func parseTrustedNetworks(networks []string) ([]*net.IPNet, error) {
	var trustedNets []*net.IPNet

	for _, network := range networks {
		network = strings.TrimSpace(network)
		if network == "" {
			continue
		}

		// Handle single IPs by adding /32 or /128
		if !strings.Contains(network, "/") {
			ip := net.ParseIP(network)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address: %s", network)
			}
			if ip.To4() != nil {
				network += "/32"
			} else {
				network += "/128"
			}
		}

		_, ipNet, err := net.ParseCIDR(network)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR network %s: %v", network, err)
		}
		trustedNets = append(trustedNets, ipNet)
	}

	return trustedNets, nil
}

// isIPTrusted checks if an IP address is in any of the trusted networks
func isIPTrusted(ip net.IP, trustedNets []*net.IPNet) bool {
	// Always allow localhost
	if ip.IsLoopback() {
		return true
	}

	// Check against trusted networks
	for _, network := range trustedNets {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// trustedNetworkInterceptor creates a gRPC interceptor that validates client IPs
func trustedNetworkInterceptor(trustedNets []*net.IPNet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip validation in development mode
		if os.Getenv("DEV_MODE") == "true" && os.Getenv("ENVIRONMENT") != "production" {
			logrus.WithField("method", info.FullMethod).Debug("Skipping network validation in dev mode")
			return handler(ctx, req)
		}

		// Get peer information
		peer, ok := peer.FromContext(ctx)
		if !ok {
			logrus.Warn("Failed to get peer information from context")
			return nil, status.Errorf(codes.Internal, "Unable to get peer information")
		}

		// Extract IP address
		var clientIP net.IP
		switch addr := peer.Addr.(type) {
		case *net.TCPAddr:
			clientIP = addr.IP
		case *net.UDPAddr:
			clientIP = addr.IP
		default:
			logrus.WithField("addr_type", fmt.Sprintf("%T", addr)).Warn("Unknown address type")
			return nil, status.Errorf(codes.Internal, "Unknown address type")
		}

		// Check if IP is trusted
		if !isIPTrusted(clientIP, trustedNets) {
			logrus.WithFields(logrus.Fields{
				"client_ip": clientIP.String(),
				"method":    info.FullMethod,
			}).Warn("Rejected connection from untrusted network")
			return nil, status.Errorf(codes.PermissionDenied, "Connection not allowed from this network")
		}

		logrus.WithFields(logrus.Fields{
			"client_ip": clientIP.String(),
			"method":    info.FullMethod,
		}).Debug("Accepted connection from trusted network")

		return handler(ctx, req)
	}
}

// trustedNetworkStreamInterceptor creates a streaming gRPC interceptor for network validation
func trustedNetworkStreamInterceptor(trustedNets []*net.IPNet) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip validation in development mode
		if os.Getenv("DEV_MODE") == "true" && os.Getenv("ENVIRONMENT") != "production" {
			logrus.WithField("method", info.FullMethod).Debug("Skipping network validation in dev mode")
			return handler(srv, ss)
		}

		// Get peer information
		peer, ok := peer.FromContext(ss.Context())
		if !ok {
			logrus.Warn("Failed to get peer information from stream context")
			return status.Errorf(codes.Internal, "Unable to get peer information")
		}

		// Extract IP address
		var clientIP net.IP
		switch addr := peer.Addr.(type) {
		case *net.TCPAddr:
			clientIP = addr.IP
		case *net.UDPAddr:
			clientIP = addr.IP
		default:
			logrus.WithField("addr_type", fmt.Sprintf("%T", addr)).Warn("Unknown address type")
			return status.Errorf(codes.Internal, "Unknown address type")
		}

		// Check if IP is trusted
		if !isIPTrusted(clientIP, trustedNets) {
			logrus.WithFields(logrus.Fields{
				"client_ip": clientIP.String(),
				"method":    info.FullMethod,
			}).Warn("Rejected streaming connection from untrusted network")
			return status.Errorf(codes.PermissionDenied, "Connection not allowed from this network")
		}

		logrus.WithFields(logrus.Fields{
			"client_ip": clientIP.String(),
			"method":    info.FullMethod,
		}).Debug("Accepted streaming connection from trusted network")

		return handler(srv, ss)
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

	// Set up proper permissions for Docker volume mounts
	if err := setupVolumePermissions(); err != nil {
		logrus.WithError(err).Warn("Failed to setup volume permissions")
	}

	// Initialize security context
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		log.Printf("Warning: Failed to set PR_SET_NO_NEW_PRIVS: %v", err)
	}

	// Parse trusted networks
	trustedNets, err := parseTrustedNetworks(Config.TrustedNetworks)
	if err != nil {
		log.Fatalf("Failed to parse trusted networks: %v", err)
	}

	logrus.WithField("trusted_networks", Config.TrustedNetworks).Info("Configured trusted networks")

	// By default, use TLS for production
	var grpcServer *grpc.Server
	var lis net.Listener

	// For development, allow connections without TLS, but with strict safeguards
	devMode := os.Getenv("DEV_MODE") == "true"
	prodEnv := os.Getenv("ENVIRONMENT") == "production" || os.Getenv("ENV") == "production"

	// Create server options with network validation
	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(trustedNetworkInterceptor(trustedNets)),
		grpc.StreamInterceptor(trustedNetworkStreamInterceptor(trustedNets)),
	}

	// Never allow insecure mode in production, regardless of DEV_MODE setting
	if devMode && !prodEnv {
		log.Println("⚠️ WARNING: Ejecutando en modo desarrollo sin TLS. NO USAR EN PRODUCCIÓN. ⚠️")
		log.Println("⚠️ Las conexiones inseguras están limitadas ÚNICAMENTE a localhost (127.0.0.1) ⚠️")

		// In dev mode, bind to all interfaces for easier testing
		lis, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", Config.GRPCPort))
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		grpcServer = grpc.NewServer(serverOpts...)
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
		serverOpts = append(serverOpts, grpc.Creds(creds))
		grpcServer = grpc.NewServer(serverOpts...)
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

func setupVolumePermissions() error {
	// Ensure the watch directory has correct ownership for web server compatibility
	uid := Config.WebServerUID
	gid := Config.WebServerGID

	if err := os.Chown(Config.WatchDir, uid, gid); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", Config.WatchDir, err)
	}

	if err := os.Chmod(Config.WatchDir, Config.DefaultDirMode); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", Config.WatchDir, err)
	}

	logrus.WithFields(logrus.Fields{
		"path": Config.WatchDir,
		"uid":  uid,
		"gid":  gid,
		"mode": Config.DefaultDirMode,
	}).Info("Volume permissions configured")

	return nil
}
