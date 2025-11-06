package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clusterv1 "github.com/mpepping/discovery-service/api/cluster/v1"
	"github.com/mpepping/discovery-service/internal/landing"
	"github.com/mpepping/discovery-service/internal/limiter"
	"github.com/mpepping/discovery-service/internal/state"
	"github.com/mpepping/discovery-service/pkg/limits"
	"github.com/mpepping/discovery-service/pkg/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	// gRPC server flags
	listenAddr       = flag.String("listen-addr", ":3000", "gRPC server listen address")
	redirectEndpoint = flag.String("redirect-endpoint", "", "Optional redirect endpoint for Hello RPC")

	// HTTP server flags
	landingAddr = flag.String("landing-addr", ":3001", "HTTP landing page listen address")
	metricsAddr = flag.String("metrics-addr", ":2122", "Prometheus metrics listen address")

	// GC flags
	gcInterval = flag.Duration("gc-interval", time.Minute, "Garbage collection interval")

	// Logging flags
	debug = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	// Initialize logger
	logger := createLogger(*debug)
	defer logger.Sync()

	logger.Info("starting discovery service",
		zap.String("listen_addr", *listenAddr),
		zap.String("landing_addr", *landingAddr),
		zap.String("metrics_addr", *metricsAddr),
		zap.Duration("gc_interval", *gcInterval),
	)

	// Create state manager
	st := state.NewState(logger)

	// Create rate limiter
	lim := limiter.NewIPLimiter()

	// Create gRPC server with logging interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(server.UnaryRequestLogger(logger)),
		grpc.ChainStreamInterceptor(server.StreamRequestLogger(logger)),
	)
	clusterServer := server.NewClusterServer(st, lim, logger, *redirectEndpoint)
	clusterv1.RegisterClusterServer(grpcServer, clusterServer)

	// Enable gRPC reflection for service discovery
	reflection.Register(grpcServer)

	// Register Prometheus collectors
	prometheus.MustRegister(st)
	prometheus.MustRegister(clusterServer)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start garbage collection
	go st.RunGC(ctx, *gcInterval)
	go lim.RunGC(ctx, limits.IPRateGarbageCollectionPeriod, 10*time.Minute)

	// Create landing page handler
	landingHandler, err := landing.NewHandler(st, logger)
	if err != nil {
		logger.Fatal("failed to create landing handler", zap.Error(err))
	}

	// Create multiplexed handler that routes gRPC and HTTP traffic
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a gRPC request
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			landingHandler.ServeHTTP(w, r)
		}
	})

	// Wrap with h2c handler to support HTTP/2 cleartext
	h2cHandler := h2c.NewHandler(mux, &http2.Server{})

	// Start main server with h2c support
	mainServer := &http.Server{
		Addr:    *listenAddr,
		Handler: h2cHandler,
	}

	go func() {
		logger.Info("server listening (gRPC + HTTP)", zap.String("addr", *listenAddr))
		if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mainServer.Shutdown(shutdownCtx)
	}()

	// Start separate landing page if configured
	if *landingAddr != "" && *landingAddr != *listenAddr {
		landingServer := &http.Server{
			Addr:    *landingAddr,
			Handler: landingHandler,
		}

		go func() {
			logger.Info("HTTP landing page listening", zap.String("addr", *landingAddr))
			if err := landingServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("landing page server error", zap.Error(err))
			}
		}()

		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			landingServer.Shutdown(shutdownCtx)
		}()
	}

	// Start Prometheus metrics server
	if *metricsAddr != "" {
		metricsServer := &http.Server{
			Addr:    *metricsAddr,
			Handler: promhttp.Handler(),
		}

		go func() {
			logger.Info("Prometheus metrics listening", zap.String("addr", *metricsAddr))
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server error", zap.Error(err))
			}
		}()

		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			metricsServer.Shutdown(shutdownCtx)
		}()
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down gracefully...")
	cancel()

	// Stop gRPC server gracefully
	grpcServer.GracefulStop()

	logger.Info("shutdown complete")
}

func createLogger(debug bool) *zap.Logger {
	config := zap.NewProductionConfig()

	if debug {
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	} else {
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// Check if running in development mode
	if os.Getenv("MODE") == "development" {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := config.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}

	return logger
}
