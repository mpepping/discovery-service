package server

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	clusterv1 "github.com/mpepping/discovery-service/api/cluster/v1"
	"github.com/mpepping/discovery-service/internal/limiter"
	"github.com/mpepping/discovery-service/internal/state"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/durationpb"
)

const bufSize = 1024 * 1024

func setupTestServer(t *testing.T) (*grpc.Server, *bufconn.Listener, clusterv1.ClusterClient) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	lim := limiter.NewIPLimiter()
	server := NewClusterServer(st, lim, logger, "")

	lis := bufconn.Listen(bufSize)

	grpcServer := grpc.NewServer()
	clusterv1.RegisterClusterServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}

	client := clusterv1.NewClusterClient(conn)

	t.Cleanup(func() {
		conn.Close()
		grpcServer.Stop()
		lis.Close()
	})

	return grpcServer, lis, client
}

func TestIntegrationHelloToUpdate(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx := context.Background()

	// Step 1: Hello
	helloResp, err := client.Hello(ctx, &clusterv1.HelloRequest{
		ClusterId:     "test-cluster",
		ClientVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if helloResp.ClientIp == "" {
		t.Log("ClientIp is empty (expected in bufconn)")
	}

	// Step 2: Update affiliate
	updateResp, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("encrypted-node-data"),
		Endpoints:   []byte("endpoint-data"),
		Ttl:         durationpb.New(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("AffiliateUpdate failed: %v", err)
	}

	if updateResp == nil {
		t.Fatal("update response is nil")
	}

	// Step 3: List affiliates
	listResp, err := client.List(ctx, &clusterv1.ListRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listResp.Affiliates) != 1 {
		t.Fatalf("expected 1 affiliate, got %d", len(listResp.Affiliates))
	}

	affiliate := listResp.Affiliates[0]
	if affiliate.Id != "node1" {
		t.Errorf("expected ID 'node1', got %q", affiliate.Id)
	}

	if string(affiliate.Data) != "encrypted-node-data" {
		t.Errorf("expected data 'encrypted-node-data', got %q", string(affiliate.Data))
	}
}

func TestIntegrationWatch(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create initial affiliate
	_, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "initial",
		Data:        []byte("initial-data"),
	})
	if err != nil {
		t.Fatalf("initial AffiliateUpdate failed: %v", err)
	}

	// Start watching
	stream, err := client.Watch(ctx, &clusterv1.WatchRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Receive snapshot
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed to receive snapshot: %v", err)
	}

	if resp.Affiliate.Id != "initial" {
		t.Errorf("expected snapshot affiliate 'initial', got %q", resp.Affiliate.Id)
	}

	if resp.Deleted {
		t.Error("snapshot affiliate should not be marked as deleted")
	}

	// Update affiliate in another goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		client.AffiliateUpdate(context.Background(), &clusterv1.AffiliateUpdateRequest{
			ClusterId:   "test-cluster",
			AffiliateId: "new-node",
			Data:        []byte("new-data"),
		})
	}()

	// Receive update
	resp, err = stream.Recv()
	if err != nil {
		t.Fatalf("failed to receive update: %v", err)
	}

	if resp.Affiliate.Id != "new-node" {
		t.Errorf("expected update affiliate 'new-node', got %q", resp.Affiliate.Id)
	}

	if resp.Deleted {
		t.Error("new affiliate should not be marked as deleted")
	}
}

func TestIntegrationWatchDelete(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create affiliate
	_, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "to-delete",
		Data:        []byte("data"),
	})
	if err != nil {
		t.Fatalf("AffiliateUpdate failed: %v", err)
	}

	// Start watching
	stream, err := client.Watch(ctx, &clusterv1.WatchRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Receive snapshot
	_, err = stream.Recv()
	if err != nil {
		t.Fatalf("failed to receive snapshot: %v", err)
	}

	// Delete affiliate
	go func() {
		time.Sleep(100 * time.Millisecond)
		client.AffiliateDelete(context.Background(), &clusterv1.AffiliateDeleteRequest{
			ClusterId:   "test-cluster",
			AffiliateId: "to-delete",
		})
	}()

	// Receive delete notification
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed to receive delete: %v", err)
	}

	if resp.Affiliate.Id != "to-delete" {
		t.Errorf("expected deleted affiliate 'to-delete', got %q", resp.Affiliate.Id)
	}

	if !resp.Deleted {
		t.Error("affiliate should be marked as deleted")
	}
}

func TestIntegrationMultipleUpdates(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx := context.Background()

	// Create multiple affiliates
	for i := 0; i < 5; i++ {
		affiliateID := string(rune('a' + i))
		_, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
			ClusterId:   "test-cluster",
			AffiliateId: affiliateID,
			Data:        []byte("data-" + affiliateID),
		})
		if err != nil {
			t.Fatalf("AffiliateUpdate %d failed: %v", i, err)
		}
	}

	// List all
	listResp, err := client.List(ctx, &clusterv1.ListRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listResp.Affiliates) != 5 {
		t.Errorf("expected 5 affiliates, got %d", len(listResp.Affiliates))
	}

	// Update existing affiliate
	_, err = client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "a",
		Data:        []byte("updated-data"),
	})
	if err != nil {
		t.Fatalf("AffiliateUpdate for existing failed: %v", err)
	}

	// List again - should still be 5
	listResp, err = client.List(ctx, &clusterv1.ListRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("second List failed: %v", err)
	}

	if len(listResp.Affiliates) != 5 {
		t.Errorf("expected 5 affiliates after update, got %d", len(listResp.Affiliates))
	}

	// Verify data was updated
	for _, a := range listResp.Affiliates {
		if a.Id == "a" && string(a.Data) != "updated-data" {
			t.Errorf("expected updated data for affiliate 'a', got %q", string(a.Data))
		}
	}
}

func TestIntegrationMultipleClusters(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx := context.Background()

	// Create affiliates in different clusters
	clusters := []string{"cluster1", "cluster2", "cluster3"}
	for _, clusterID := range clusters {
		_, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
			ClusterId:   clusterID,
			AffiliateId: "node1",
			Data:        []byte("data-" + clusterID),
		})
		if err != nil {
			t.Fatalf("AffiliateUpdate for %s failed: %v", clusterID, err)
		}
	}

	// Verify isolation - each cluster should have exactly 1 affiliate
	for _, clusterID := range clusters {
		listResp, err := client.List(ctx, &clusterv1.ListRequest{
			ClusterId: clusterID,
		})
		if err != nil {
			t.Fatalf("List for %s failed: %v", clusterID, err)
		}

		if len(listResp.Affiliates) != 1 {
			t.Errorf("expected 1 affiliate in %s, got %d", clusterID, len(listResp.Affiliates))
		}

		expectedData := "data-" + clusterID
		if string(listResp.Affiliates[0].Data) != expectedData {
			t.Errorf("expected data %q in %s, got %q", expectedData, clusterID, string(listResp.Affiliates[0].Data))
		}
	}
}

func TestIntegrationWatchCancellation(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	// Start watching
	stream, err := client.Watch(ctx, &clusterv1.WatchRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Cancel context
	cancel()

	// Receive should fail
	_, err = stream.Recv()
	if err == nil {
		t.Error("expected error after context cancel")
	}

	if err != io.EOF && err != context.Canceled {
		t.Logf("received error: %v", err)
	}
}

func TestIntegrationEndpointMerging(t *testing.T) {
	_, _, client := setupTestServer(t)

	ctx := context.Background()

	// First update with endpoints
	_, err := client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Data:        []byte("data"),
		Endpoints:   []byte("endpoint1"),
	})
	if err != nil {
		t.Fatalf("first AffiliateUpdate failed: %v", err)
	}

	// Second update with more endpoints
	_, err = client.AffiliateUpdate(ctx, &clusterv1.AffiliateUpdateRequest{
		ClusterId:   "test-cluster",
		AffiliateId: "node1",
		Endpoints:   []byte("endpoint2"),
	})
	if err != nil {
		t.Fatalf("second AffiliateUpdate failed: %v", err)
	}

	// List and verify endpoints were merged
	listResp, err := client.List(ctx, &clusterv1.ListRequest{
		ClusterId: "test-cluster",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listResp.Affiliates) != 1 {
		t.Fatalf("expected 1 affiliate, got %d", len(listResp.Affiliates))
	}

	// Endpoints should be combined
	endpoints := listResp.Affiliates[0].Endpoints
	expectedLen := len("endpoint1") + len("endpoint2")
	if len(endpoints) != expectedLen {
		t.Errorf("expected combined endpoints length %d, got %d", expectedLen, len(endpoints))
	}
}
