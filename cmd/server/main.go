package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/gitops-ci-cd/greeting-service/internal/services"
)

func init() {
	level := func() slog.Level {
		switch os.Getenv("LOG_LEVEL") {
		case "ERROR":
			return slog.LevelError
		case "WARN":
			return slog.LevelWarn
		case "INFO":
			return slog.LevelInfo
		case "DEBUG":
			return slog.LevelDebug
		default:
			return slog.LevelInfo
		}
	}()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

func main() {
	port := ":" + os.Getenv("PORT")
	if port == ":" {
		port = ":50051"
	}

	// Run the server
	if err := run(port, services.Register); err != nil {
		slog.Error("Server terminated", "error", err)
		os.Exit(1)
	} else {
		slog.Warn("Server stopped")
	}
}

// run sets up and starts the gRPC server
func run(port string, registerFunc func(*grpc.Server)) error {
	// Create a TCP listener
	listener, err := net.Listen("tcp", port)
	if err != nil {
		slog.Error("Failed to start listener", "port", port, "error", err)
		return err
	}
	defer listener.Close()

	// Create a new gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor),
	)

	// Register services using the provided function
	registerFunc(server)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	setupSignalHandler(cancel)

	// Run the server in a goroutine to allow for graceful shutdown
	go func() {
		slog.Info("Server listening...", "port", port)
		if err := server.Serve(listener); err != nil {
			slog.Error("Failed to serve", "error", err)
			cancel()
		}
	}()

	// Wait for termination signal
	<-ctx.Done()
	slog.Warn("Server shutting down gracefully...")
	server.GracefulStop()

	return nil
}

// loggingInterceptor logs all incoming gRPC requests
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	if protoReq, ok := req.(proto.Message); ok {
		// Serialize protobuf message to JSON for logging
		reqJSON, err := protojson.Marshal(protoReq)
		if err != nil {
			slog.Debug("Failed to marshal request to JSON", "error", err)
		} else {
			slog.Debug("gRPC request received", "method", info.FullMethod, "request", reqJSON)
		}
	}

	// Process the request
	res, err := handler(ctx, req)
	duration := time.Since(start)

	fields := []any{
		"method", info.FullMethod,
		"duration", duration.String(),
	}

	if err != nil {
		fields = append(fields, "error", err)
	}

	slog.Info("Handled gRPC request", fields...)

	return res, err
}

// setupSignalHandler sets up a signal handler to cancel the provided context
func setupSignalHandler(cancelFunc context.CancelFunc) {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		sig := <-ch
		slog.Warn("Received termination signal", "signal", sig)
		cancelFunc()
	}()
}
