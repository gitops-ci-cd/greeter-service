package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/gitops-ci-cd/greeting-service/internal/services"
	"github.com/gitops-ci-cd/greeting-service/pkg/telemetry"
)

// Configure the logger
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

// main is the entry point for the server
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	// Run the server
	if err := run(":"+port, services.Register); err != nil {
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
		return fmt.Errorf("could not create tcp listener on port %s: %w", port, err)
	}
	defer listener.Close()

	// Create a new gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(telemetry.LoggingInterceptor),
	)

	// Register services using the provided function
	registerFunc(server)

	// Run the server in a goroutine to allow for graceful shutdown
	ctx := setupSignalHandler()
	go func() {
		slog.Info("Server listening...", "port", port)
		if err := server.Serve(listener); err != nil {
			slog.Error("gRPC server failed", "error", err)
		}
	}()

	// Wait for termination signal
	<-ctx.Done()
	slog.Warn("Server shutting down gracefully...")
	server.GracefulStop()

	return nil
}

// setupSignalHandler creates a cancellable context for signal handling
func setupSignalHandler() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		slog.Warn("Received termination signal")
		stop()
	}()
	return ctx
}
