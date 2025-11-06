package server

import (
	"context"
	"net"
	"testing"
	"time"

	clusterv1 "github.com/mpepping/discovery-service/api/cluster/v1"
	"github.com/mpepping/discovery-service/internal/limiter"
	"github.com/mpepping/discovery-service/internal/state"
	"github.com/mpepping/discovery-service/pkg/limits"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewClusterServer(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()

	server := NewClusterServer(st, lim, logger, "")

	if server == nil {
		t.Fatal("NewClusterServer returned nil")
	}

	if server.state != st {
		t.Error("state not set correctly")
	}

	if server.limiter != lim {
		t.Error("limiter not set correctly")
	}

	if server.logger != logger {
		t.Error("logger not set correctly")
	}

	if server.helloRequests == nil {
		t.Error("helloRequests metric not initialized")
	}
}

func TestHello(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.HelloRequest{
		ClusterId:     "test-cluster",
		ClientVersion: "1.0.0",
	}

	resp, err := server.Hello(ctx, req)
	if err != nil {
		t.Fatalf("Hello returned error: %v", err)
	}

	if resp == nil {
		t.Fatal("Hello returned nil response")
	}

	if resp.Redirect != nil {
		t.Error("expected no redirect when not configured")
	}
}

func TestHelloWithRedirect(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	redirectEndpoint := "discovery.example.com:3000"
	server := NewClusterServer(st, lim, logger, redirectEndpoint)

	ctx := context.Background()
	req := &clusterv1.HelloRequest{
		ClusterId:     "test-cluster",
		ClientVersion: "1.0.0",
	}

	resp, err := server.Hello(ctx, req)
	if err != nil {
		t.Fatalf("Hello returned error: %v", err)
	}

	if resp.Redirect == nil {
		t.Fatal("expected redirect but got none")
	}

	if resp.Redirect.Addr != redirectEndpoint {
		t.Errorf("expected redirect to %q, got %q", redirectEndpoint, resp.Redirect.Addr)
	}
}

func TestHelloMissingClusterID(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.HelloRequest{
		ClientVersion: "1.0.0",
	}

	_, err := server.Hello(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing cluster_id")
	}

	statusErr, ok := status.FromError(err)
	if !ok {
		t.Fatal("error is not a status error")
	}

	if statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", statusErr.Code())
	}
}

func TestHelloWithPeerIP(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	// Create context with peer info
	peerAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.1"),
		Port: 12345,
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: peerAddr,
	})

	req := &clusterv1.HelloRequest{
		ClusterId:     "test-cluster",
		ClientVersion: "1.0.0",
	}

	resp, err := server.Hello(ctx, req)
	if err != nil {
		t.Fatalf("Hello returned error: %v", err)
	}

	if resp.ClientIp != "192.168.1.1" {
		t.Errorf("expected client IP '192.168.1.1', got %q", resp.ClientIp)
	}
}

func TestAffiliateUpdate(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("encrypted-data"),
		Ttl:         durationpb.New(30 * time.Minute),
	}

	resp, err := server.AffiliateUpdate(ctx, req)
	if err != nil {
		t.Fatalf("AffiliateUpdate returned error: %v", err)
	}

	if resp == nil {
		t.Fatal("AffiliateUpdate returned nil response")
	}

	// Verify affiliate was created
	cluster := st.GetCluster("test-cluster")
	affiliates := cluster.ListAffiliates()

	if len(affiliates) != 1 {
		t.Fatalf("expected 1 affiliate, got %d", len(affiliates))
	}

	if affiliates[0].ID != "node1" {
		t.Errorf("expected affiliate ID 'node1', got %q", affiliates[0].ID)
	}

	if string(affiliates[0].Data) != "encrypted-data" {
		t.Errorf("expected data 'encrypted-data', got %q", string(affiliates[0].Data))
	}
}

func TestAffiliateUpdateWithEndpoints(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("data"),
		Endpoints:   []byte("endpoint-data"),
		Ttl:         durationpb.New(30 * time.Minute),
	}

	_, err := server.AffiliateUpdate(ctx, req)
	if err != nil {
		t.Fatalf("AffiliateUpdate returned error: %v", err)
	}

	// Verify endpoints were added
	cluster := st.GetCluster("test-cluster")
	affiliates := cluster.ListAffiliates()

	if len(affiliates[0].Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(affiliates[0].Endpoints))
	}

	if string(affiliates[0].Endpoints[0].Data) != "endpoint-data" {
		t.Errorf("expected endpoint data 'endpoint-data', got %q", string(affiliates[0].Endpoints[0].Data))
	}
}

func TestAffiliateUpdateMissingClusterID(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		AffiliateId: "node1",
		Data:        []byte("data"),
	}

	_, err := server.AffiliateUpdate(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing cluster_id")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error")
	}
}

func TestAffiliateUpdateMissingAffiliateID(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		ClusterId: "test-cluster",
		Data:      []byte("data"),
	}

	_, err := server.AffiliateUpdate(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing affiliate_id")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error")
	}
}

func TestAffiliateUpdateNoDataOrEndpoints(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
	}

	_, err := server.AffiliateUpdate(ctx, req)
	if err == nil {
		t.Fatal("expected error when both data and endpoints are missing")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error")
	}
}

func TestAffiliateUpdateDefaultTTL(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("data"),
		// No TTL specified
	}

	_, err := server.AffiliateUpdate(ctx, req)
	if err != nil {
		t.Fatalf("AffiliateUpdate returned error: %v", err)
	}

	// Verify affiliate expiration is around 30 minutes from now
	cluster := st.GetCluster("test-cluster")
	affiliates := cluster.ListAffiliates()

	expectedExpiration := time.Now().Add(30 * time.Minute)
	actualExpiration := affiliates[0].Expiration

	diff := actualExpiration.Sub(expectedExpiration)
	if diff < -1*time.Second || diff > 1*time.Second {
		t.Errorf("expected expiration around %v, got %v (diff: %v)", expectedExpiration, actualExpiration, diff)
	}
}

func TestAffiliateDelete(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()

	// Create affiliate first
	_, err := server.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("data"),
	})
	if err != nil {
		t.Fatalf("AffiliateUpdate failed: %v", err)
	}

	// Delete it
	resp, err := server.AffiliateDelete(ctx, &clusterv1.AffiliateDeleteRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
	})
	if err != nil {
		t.Fatalf("AffiliateDelete returned error: %v", err)
	}

	if resp == nil {
		t.Fatal("AffiliateDelete returned nil response")
	}

	// Verify it's gone
	cluster := st.GetCluster("test-cluster")
	affiliates := cluster.ListAffiliates()

	if len(affiliates) != 0 {
		t.Errorf("expected 0 affiliates after delete, got %d", len(affiliates))
	}
}

func TestAffiliateDeleteMissingClusterID(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateDeleteRequest{
		AffiliateId: "node1",
	}

	_, err := server.AffiliateDelete(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing cluster_id")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error")
	}
}

func TestAffiliateDeleteNonexistent(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	req := &clusterv1.AffiliateDeleteRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "nonexistent",
	}

	// Should not error
	_, err := server.AffiliateDelete(ctx, req)
	if err != nil {
		t.Errorf("AffiliateDelete should not error for nonexistent affiliate: %v", err)
	}
}

func TestList(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()

	// Create multiple affiliates
	affiliateIDs := []string{"node1", "node2", "node3"}
	for _, id := range affiliateIDs {
		_, err := server.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
			ClusterId:   "test-cluster",
			AffiliateId: id,
			Data:        []byte("data-" + id),
		})
		if err != nil {
			t.Fatalf("AffiliateUpdate failed: %v", err)
		}
	}

	// List affiliates
	resp, err := server.List(ctx, &clusterv1.ListRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(resp.Affiliates) != len(affiliateIDs) {
		t.Errorf("expected %d affiliates, got %d", len(affiliateIDs), len(resp.Affiliates))
	}

	// Verify all IDs are present
	foundIDs := make(map[string]bool)
	for _, a := range resp.Affiliates {
		foundIDs[a.Id] = true
	}

	for _, id := range affiliateIDs {
		if !foundIDs[id] {
			t.Errorf("affiliate %q not found in list", id)
		}
	}
}

func TestListEmpty(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	resp, err := server.List(ctx, &clusterv1.ListRequest{
		ClusterId: "empty-cluster",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(resp.Affiliates) != 0 {
		t.Errorf("expected 0 affiliates for empty cluster, got %d", len(resp.Affiliates))
	}
}

func TestListMissingClusterID(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	ctx := context.Background()
	_, err := server.List(ctx, &clusterv1.ListRequest{})
	if err == nil {
		t.Fatal("expected error for missing cluster_id")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error")
	}
}

func TestRateLimiting(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	// Create context with peer IP
	peerAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.1"),
		Port: 12345,
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: peerAddr,
	})

	req := &clusterv1.HelloRequest{
		ClusterId:     "test-cluster",
		ClientVersion: "1.0.0",
	}

	// Exhaust rate limit
	for i := 0; i < limits.IPRateBurstSizeMax; i++ {
		_, err := server.Hello(ctx, req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	// Next request should be rate limited
	_, err := server.Hello(ctx, req)
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	statusErr, ok := status.FromError(err)
	if !ok || statusErr.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted error, got %v", err)
	}
}
