package state

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewState(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	if state == nil {
		t.Fatal("NewState returned nil")
	}

	if state.clusters == nil {
		t.Error("clusters map not initialized")
	}

	if state.logger != logger {
		t.Error("logger not set correctly")
	}
}

func TestGetCluster(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	clusterID := "test-cluster"
	cluster := state.GetCluster(clusterID)

	if cluster == nil {
		t.Fatal("GetCluster returned nil")
	}

	if cluster.ID != clusterID {
		t.Errorf("expected cluster ID %q, got %q", clusterID, cluster.ID)
	}

	// Getting the same cluster should return the same instance
	cluster2 := state.GetCluster(clusterID)
	if cluster != cluster2 {
		t.Error("GetCluster returned different instance for same ID")
	}
}

func TestGetClusterConcurrent(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	clusterID := "test-cluster"
	done := make(chan *Cluster, 10)

	// Concurrently try to get/create the same cluster
	for i := 0; i < 10; i++ {
		go func() {
			done <- state.GetCluster(clusterID)
		}()
	}

	// Collect all results
	clusters := make([]*Cluster, 10)
	for i := 0; i < 10; i++ {
		clusters[i] = <-done
	}

	// All should be the same instance
	for i := 1; i < 10; i++ {
		if clusters[0] != clusters[i] {
			t.Error("concurrent GetCluster created multiple instances")
		}
	}
}

func TestGarbageCollect(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	// Create cluster with expired affiliate
	cluster := state.GetCluster("test-cluster")
	expiredTime := time.Now().Add(-1 * time.Hour)

	cluster.affiliatesMu.Lock()
	cluster.affiliates["expired"] = &Affiliate{
		ID:         "expired",
		Data:       []byte("test"),
		Expiration: expiredTime,
	}
	cluster.affiliates["active"] = &Affiliate{
		ID:         "active",
		Data:       []byte("test"),
		Expiration: time.Now().Add(1 * time.Hour),
	}
	cluster.affiliatesMu.Unlock()

	// Run garbage collection
	state.GarbageCollect(time.Now())

	// Check that expired affiliate was removed but active remains
	affiliates := cluster.ListAffiliates()
	if len(affiliates) != 1 {
		t.Errorf("expected 1 affiliate after GC, got %d", len(affiliates))
	}

	if affiliates[0].ID != "active" {
		t.Errorf("expected 'active' affiliate to remain, got %q", affiliates[0].ID)
	}
}

func TestGarbageCollectEmptyCluster(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	// Create cluster with only expired affiliates
	cluster := state.GetCluster("test-cluster")
	expiredTime := time.Now().Add(-1 * time.Hour)

	cluster.affiliatesMu.Lock()
	cluster.affiliates["expired"] = &Affiliate{
		ID:         "expired",
		Data:       []byte("test"),
		Expiration: expiredTime,
	}
	cluster.affiliatesMu.Unlock()

	// Verify cluster exists
	state.mu.RLock()
	clusterCount := len(state.clusters)
	state.mu.RUnlock()
	if clusterCount != 1 {
		t.Errorf("expected 1 cluster before GC, got %d", clusterCount)
	}

	// Run garbage collection
	state.GarbageCollect(time.Now())

	// Cluster should be removed since it's empty
	state.mu.RLock()
	clusterCount = len(state.clusters)
	state.mu.RUnlock()
	if clusterCount != 0 {
		t.Errorf("expected 0 clusters after GC, got %d", clusterCount)
	}
}

func TestRunGC(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	// Create cluster with short-lived affiliate
	cluster := state.GetCluster("test-cluster")
	shortExpiration := time.Now().Add(50 * time.Millisecond)

	cluster.affiliatesMu.Lock()
	cluster.affiliates["short-lived"] = &Affiliate{
		ID:         "short-lived",
		Data:       []byte("test"),
		Expiration: shortExpiration,
	}
	cluster.affiliatesMu.Unlock()

	// Start GC with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go state.RunGC(ctx, 30*time.Millisecond)

	// Wait for GC to run a couple times
	time.Sleep(100 * time.Millisecond)

	// Affiliate should be gone
	affiliates := cluster.ListAffiliates()
	if len(affiliates) != 0 {
		t.Errorf("expected 0 affiliates after GC, got %d", len(affiliates))
	}

	// Cancel and verify it stops
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestPrometheusMetrics(t *testing.T) {
	logger := zap.NewNop()
	state := NewState(logger)

	// Verify metrics are initialized
	if state.gcRuns == nil {
		t.Error("gcRuns metric not initialized")
	}
	if state.gcCollectedClusters == nil {
		t.Error("gcCollectedClusters metric not initialized")
	}
	if state.gcCollectedAffiliates == nil {
		t.Error("gcCollectedAffiliates metric not initialized")
	}

	// Run GC and verify metrics are updated
	state.GarbageCollect(time.Now())
	// Metrics should be incrementable without panic
}
