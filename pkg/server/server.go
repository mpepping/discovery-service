package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	clusterv1 "github.com/mpepping/discovery-service/api/cluster/v1"
	"github.com/mpepping/discovery-service/internal/limiter"
	"github.com/mpepping/discovery-service/internal/state"
	"github.com/mpepping/discovery-service/pkg/limits"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ClusterServer implements the Cluster gRPC service
type ClusterServer struct {
	clusterv1.UnimplementedClusterServer

	state            *state.State
	limiter          *limiter.IPLimiter
	logger           *zap.Logger
	redirectEndpoint string

	// Metrics
	helloRequests *prometheus.CounterVec
}

// NewClusterServer creates a new cluster server
func NewClusterServer(st *state.State, lim *limiter.IPLimiter, logger *zap.Logger, redirectEndpoint string) *ClusterServer {
	return &ClusterServer{
		state:            st,
		limiter:          lim,
		logger:           logger,
		redirectEndpoint: redirectEndpoint,
		helloRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "discovery_hello_requests_total",
				Help: "Total number of hello requests by client version",
			},
			[]string{"client_version"},
		),
	}
}

// Hello implements the Hello RPC method
func (s *ClusterServer) Hello(ctx context.Context, req *clusterv1.HelloRequest) (*clusterv1.HelloResponse, error) {
	// Extract peer IP
	clientIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
	}

	// Rate limiting
	if clientIP != "" && !s.limiter.Allow(clientIP) {
		s.logger.Warn("rate limit exceeded",
			zap.String("client_ip", clientIP),
		)
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Validate cluster ID
	if req.ClusterId == "" {
		s.logger.Warn("hello request missing cluster_id",
			zap.String("client_ip", clientIP),
		)
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	// Record metrics
	s.helloRequests.WithLabelValues(req.ClientVersion).Inc()

	resp := &clusterv1.HelloResponse{
		ClientIp: clientIP,
	}

	// Add redirect if configured
	if s.redirectEndpoint != "" {
		resp.Redirect = &clusterv1.Endpoint{
			Addr: s.redirectEndpoint,
		}
		s.logger.Debug("hello response with redirect",
			zap.String("client_ip", clientIP),
			zap.String("redirect", s.redirectEndpoint),
		)
	} else {
		s.logger.Debug("hello response",
			zap.String("client_ip", clientIP),
		)
	}

	return resp, nil
}

// AffiliateUpdate implements the AffiliateUpdate RPC method
func (s *ClusterServer) AffiliateUpdate(ctx context.Context, req *clusterv1.AffiliateUpdateRequest) (*clusterv1.AffiliateUpdateResponse, error) {
	s.logger.Debug("AffiliateUpdate called",
		zap.String("cluster_id", req.ClusterId),
		zap.String("affiliate_id", req.AffiliateId),
		zap.Int("data_len", len(req.Data)),
		zap.Int("endpoints_len", len(req.Endpoints)),
	)

	// Extract peer IP for rate limiting
	clientIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
	}

	// Rate limiting
	if clientIP != "" && !s.limiter.Allow(clientIP) {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Validate request
	if req.ClusterId == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}
	if req.AffiliateId == "" {
		return nil, status.Error(codes.InvalidArgument, "affiliate_id is required")
	}
	if len(req.Data) == 0 && len(req.Endpoints) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one of data or endpoints is required")
	}
	if req.Ttl != nil && req.Ttl.AsDuration() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "ttl must be positive")
	}

	// Default TTL is 30 minutes
	ttl := 30 * time.Minute
	if req.Ttl != nil {
		ttl = req.Ttl.AsDuration()
	}
	expiration := time.Now().Add(ttl)

	// Get or create cluster
	cluster := s.state.GetCluster(req.ClusterId)

	// Update affiliate and get the actual affiliate reference for notification
	var affiliateToNotify *state.Affiliate
	err := cluster.WithAffiliate(req.AffiliateId, func(affiliate *state.Affiliate) error {
		// Update data if provided
		if len(req.Data) > 0 {
			s.logger.Debug("updating affiliate data",
				zap.String("cluster_id", req.ClusterId),
				zap.String("affiliate_id", req.AffiliateId),
				zap.Int("data_len", len(req.Data)),
			)
			affiliate.Update(req.Data, expiration)
		}

		// Update endpoints if provided
		if len(req.Endpoints) > 0 {
			s.logger.Debug("updating affiliate endpoints",
				zap.String("cluster_id", req.ClusterId),
				zap.String("affiliate_id", req.AffiliateId),
				zap.Int("endpoints_len", len(req.Endpoints)),
			)
			endpoints := []state.EndpointEntry{
				{
					Data:       req.Endpoints,
					Expiration: expiration,
				},
			}
			affiliate.MergeEndpoints(endpoints)
		}

		// Store reference for notification
		affiliateToNotify = affiliate
		return nil
	})

	if err != nil {
		if errors.Is(err, state.ErrAffiliateLimit) {
			s.logger.Warn("affiliate limit reached",
				zap.String("cluster_id", req.ClusterId),
				zap.String("affiliate_id", req.AffiliateId),
			)
			return nil, status.Error(codes.ResourceExhausted, "affiliate limit reached for cluster")
		}
		s.logger.Error("failed to update affiliate",
			zap.String("cluster_id", req.ClusterId),
			zap.String("affiliate_id", req.AffiliateId),
			zap.Error(err),
		)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to update affiliate: %v", err))
	}

	// Notify subscribers with the actual affiliate reference
	if affiliateToNotify != nil {
		cluster.NotifyUpdate(affiliateToNotify)
	}

	return &clusterv1.AffiliateUpdateResponse{}, nil
}

// AffiliateDelete implements the AffiliateDelete RPC method
func (s *ClusterServer) AffiliateDelete(ctx context.Context, req *clusterv1.AffiliateDeleteRequest) (*clusterv1.AffiliateDeleteResponse, error) {
	// Extract peer IP for rate limiting
	clientIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
	}

	// Rate limiting
	if clientIP != "" && !s.limiter.Allow(clientIP) {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Validate request
	if req.ClusterId == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}
	if req.AffiliateId == "" {
		return nil, status.Error(codes.InvalidArgument, "affiliate_id is required")
	}

	// Get cluster
	cluster := s.state.GetCluster(req.ClusterId)

	// Delete affiliate
	cluster.DeleteAffiliate(req.AffiliateId)

	return &clusterv1.AffiliateDeleteResponse{}, nil
}

// List implements the List RPC method
func (s *ClusterServer) List(ctx context.Context, req *clusterv1.ListRequest) (*clusterv1.ListResponse, error) {
	// Extract peer IP for rate limiting
	clientIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
	}

	// Rate limiting
	if clientIP != "" && !s.limiter.Allow(clientIP) {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Validate request
	if req.ClusterId == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	// Get cluster
	cluster := s.state.GetCluster(req.ClusterId)

	// List affiliates
	affiliates := cluster.ListAffiliates()

	s.logger.Debug("listing affiliates",
		zap.String("cluster_id", req.ClusterId),
		zap.Int("count", len(affiliates)),
		zap.String("client_ip", clientIP),
	)

	// Convert to proto
	protoAffiliates := make([]*clusterv1.Affiliate, 0, len(affiliates))
	for _, a := range affiliates {
		// Combine all endpoint data into a single blob
		var endpointsData []byte
		for _, ep := range a.Endpoints {
			endpointsData = append(endpointsData, ep.Data...)
		}

		s.logger.Debug("affiliate details",
			zap.String("cluster_id", req.ClusterId),
			zap.String("affiliate_id", a.ID),
			zap.Int("data_len", len(a.Data)),
			zap.Int("endpoints_count", len(a.Endpoints)),
			zap.Int("endpoints_data_len", len(endpointsData)),
		)

		protoAffiliates = append(protoAffiliates, &clusterv1.Affiliate{
			Id:        a.ID,
			Data:      a.Data,
			Endpoints: endpointsData,
		})
	}

	return &clusterv1.ListResponse{
		Affiliates: protoAffiliates,
	}, nil
}

// Watch implements the Watch RPC method
func (s *ClusterServer) Watch(req *clusterv1.WatchRequest, stream clusterv1.Cluster_WatchServer) error {
	// Validate request
	if req.ClusterId == "" {
		return status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	// Get cluster
	cluster := s.state.GetCluster(req.ClusterId)

	// Subscribe to changes
	snapshot, subscription := cluster.Subscribe()
	defer cluster.Unsubscribe(subscription)

	s.logger.Debug("watch started",
		zap.String("cluster_id", req.ClusterId),
		zap.Int("snapshot_count", len(snapshot)),
	)

	// Send snapshot
	for _, a := range snapshot {
		// Combine all endpoint data
		var endpointsData []byte
		for _, ep := range a.Endpoints {
			endpointsData = append(endpointsData, ep.Data...)
		}

		s.logger.Debug("sending watch snapshot",
			zap.String("cluster_id", req.ClusterId),
			zap.String("affiliate_id", a.ID),
			zap.Int("data_len", len(a.Data)),
			zap.Int("endpoints_count", len(a.Endpoints)),
		)

		err := stream.Send(&clusterv1.WatchResponse{
			Affiliate: &clusterv1.Affiliate{
				Id:        a.ID,
				Data:      a.Data,
				Endpoints: endpointsData,
			},
			Deleted: false,
		})
		if err != nil {
			s.logger.Debug("watch stream error during snapshot",
				zap.String("cluster_id", req.ClusterId),
				zap.Error(err),
			)
			return err
		}
	}

	// Stream updates
	for {
		select {
		case <-stream.Context().Done():
			s.logger.Debug("watch ended",
				zap.String("cluster_id", req.ClusterId),
			)
			return stream.Context().Err()

		case notification, ok := <-subscription.Ch():
			if !ok {
				// Subscription closed
				s.logger.Debug("watch subscription closed",
					zap.String("cluster_id", req.ClusterId),
				)
				return nil
			}

			// Combine endpoint data
			var endpointsData []byte
			for _, ep := range notification.Affiliate.Endpoints {
				endpointsData = append(endpointsData, ep.Data...)
			}

			s.logger.Debug("sending watch update",
				zap.String("cluster_id", req.ClusterId),
				zap.String("affiliate_id", notification.Affiliate.ID),
				zap.Bool("deleted", notification.Deleted),
				zap.Int("data_len", len(notification.Affiliate.Data)),
				zap.Int("endpoints_count", len(notification.Affiliate.Endpoints)),
			)

			err := stream.Send(&clusterv1.WatchResponse{
				Affiliate: &clusterv1.Affiliate{
					Id:        notification.Affiliate.ID,
					Data:      notification.Affiliate.Data,
					Endpoints: endpointsData,
				},
				Deleted: notification.Deleted,
			})
			if err != nil {
				s.logger.Debug("watch stream error",
					zap.String("cluster_id", req.ClusterId),
					zap.Error(err),
				)
				return err
			}
		}
	}
}

// Describe implements prometheus.Collector
func (s *ClusterServer) Describe(ch chan<- *prometheus.Desc) {
	s.helloRequests.Describe(ch)
}

// Collect implements prometheus.Collector
func (s *ClusterServer) Collect(ch chan<- prometheus.Metric) {
	s.helloRequests.Collect(ch)
}

// Default TTL for compatibility
var _ = durationpb.Duration{}
var _ = limits.ClusterAffiliatesMax
