package server

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// UnaryRequestLogger creates a gRPC unary interceptor for logging requests
func UnaryRequestLogger(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Execute the handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Extract peer address
		peerAddr := ""
		if p, ok := peer.FromContext(ctx); ok {
			peerAddr = p.Addr.String()
		}

		// Extract fields from request
		fields := []zap.Field{
			zap.Duration("duration", duration),
			zap.String("code", status.Code(err).String()),
			zap.String("peer.address", peerAddr),
		}

		// Extract common fields from request
		fields = append(fields, extractFields(req)...)

		// Log the request
		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.Error(info.FullMethod, fields...)
		} else {
			logger.Info(info.FullMethod, fields...)
		}

		return resp, err
	}
}

// extractFields extracts common fields from request messages
func extractFields(req interface{}) []zap.Field {
	fields := make([]zap.Field, 0, 3)

	// Try to extract cluster_id
	if reqWithCluster, ok := req.(interface{ GetClusterId() string }); ok {
		if clusterID := reqWithCluster.GetClusterId(); clusterID != "" {
			fields = append(fields, zap.String("cluster_id", clusterID))
		}
	}

	// Try to extract affiliate_id
	if reqWithAffiliate, ok := req.(interface{ GetAffiliateId() string }); ok {
		if affiliateID := reqWithAffiliate.GetAffiliateId(); affiliateID != "" {
			fields = append(fields, zap.String("affiliate_id", affiliateID))
		}
	}

	// Try to extract client_version
	if reqWithVersion, ok := req.(interface{ GetClientVersion() string }); ok {
		if version := reqWithVersion.GetClientVersion(); version != "" {
			fields = append(fields, zap.String("client_version", version))
		}
	}

	return fields
}

// StreamRequestLogger creates a gRPC stream interceptor for logging requests
func StreamRequestLogger(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		// Execute the handler
		err := handler(srv, ss)

		// Calculate duration
		duration := time.Since(start)

		// Extract peer address
		peerAddr := ""
		if p, ok := peer.FromContext(ss.Context()); ok {
			peerAddr = p.Addr.String()
		}

		// Log the request
		fields := []zap.Field{
			zap.Duration("duration", duration),
			zap.String("code", status.Code(err).String()),
			zap.String("peer.address", peerAddr),
		}

		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.Error(info.FullMethod, fields...)
		} else {
			logger.Info(info.FullMethod, fields...)
		}

		return err
	}
}
