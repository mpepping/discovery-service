package state

import (
	"testing"
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
	"go.uber.org/zap"
)

func TestNewCluster(t *testing.T) {
	logger := zap.NewNop()
	clusterID := "test-cluster"
	cluster := NewCluster(clusterID, logger)

	if cluster == nil {
		t.Fatal("NewCluster returned nil")
	}

	if cluster.ID != clusterID {
		t.Errorf("expected ID %q, got %q", clusterID, cluster.ID)
	}

	if cluster.affiliates == nil {
		t.Error("affiliates map not initialized")
	}

	if len(cluster.subscriptions) != 0 {
		t.Error("subscriptions should be empty initially")
	}
}

func TestWithAffiliate(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	affiliateID := "test-affiliate"
	called := false

	err := cluster.WithAffiliate(affiliateID, func(a *Affiliate) error {
		called = true
		if a.ID != affiliateID {
			t.Errorf("expected affiliate ID %q, got %q", affiliateID, a.ID)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithAffiliate returned error: %v", err)
	}

	if !called {
		t.Error("callback was not called")
	}

	// Verify affiliate was created
	cluster.affiliatesMu.Lock()
	affiliate, exists := cluster.affiliates[affiliateID]
	cluster.affiliatesMu.Unlock()

	if !exists {
		t.Error("affiliate was not created")
	}

	if affiliate.ID != affiliateID {
		t.Errorf("expected affiliate ID %q, got %q", affiliateID, affiliate.ID)
	}
}

func TestWithAffiliateExisting(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	affiliateID := "test-affiliate"

	// Create affiliate
	err := cluster.WithAffiliate(affiliateID, func(a *Affiliate) error {
		a.Data = []byte("first")
		return nil
	})
	if err != nil {
		t.Fatalf("first WithAffiliate returned error: %v", err)
	}

	// Update same affiliate
	err = cluster.WithAffiliate(affiliateID, func(a *Affiliate) error {
		if string(a.Data) != "first" {
			t.Error("expected to receive existing affiliate with data")
		}
		a.Data = []byte("second")
		return nil
	})
	if err != nil {
		t.Fatalf("second WithAffiliate returned error: %v", err)
	}

	// Verify update
	cluster.affiliatesMu.Lock()
	affiliate := cluster.affiliates[affiliateID]
	cluster.affiliatesMu.Unlock()

	if string(affiliate.Data) != "second" {
		t.Errorf("expected data 'second', got %q", string(affiliate.Data))
	}
}

func TestWithAffiliateLimit(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Fill up to limit
	for i := 0; i < limits.ClusterAffiliatesMax; i++ {
		affiliateID := string(rune('a' + i))
		err := cluster.WithAffiliate(affiliateID, func(a *Affiliate) error {
			return nil
		})
		if err != nil {
			t.Fatalf("WithAffiliate failed at %d: %v", i, err)
		}
	}

	// Next one should fail
	err := cluster.WithAffiliate("overflow", func(a *Affiliate) error {
		t.Error("callback should not be called when limit is reached")
		return nil
	})

	if err != ErrAffiliateLimit {
		t.Errorf("expected ErrAffiliateLimit, got %v", err)
	}
}

func TestDeleteAffiliate(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	affiliateID := "test-affiliate"

	// Create affiliate
	err := cluster.WithAffiliate(affiliateID, func(a *Affiliate) error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithAffiliate returned error: %v", err)
	}

	// Verify it exists
	cluster.affiliatesMu.Lock()
	_, exists := cluster.affiliates[affiliateID]
	cluster.affiliatesMu.Unlock()
	if !exists {
		t.Fatal("affiliate was not created")
	}

	// Delete it
	cluster.DeleteAffiliate(affiliateID)

	// Verify it's gone
	cluster.affiliatesMu.Lock()
	_, exists = cluster.affiliates[affiliateID]
	cluster.affiliatesMu.Unlock()
	if exists {
		t.Error("affiliate was not deleted")
	}
}

func TestDeleteNonexistentAffiliate(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Should not panic
	cluster.DeleteAffiliate("nonexistent")
}

func TestListAffiliates(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create multiple affiliates
	affiliateIDs := []string{"affiliate1", "affiliate2", "affiliate3"}
	for _, id := range affiliateIDs {
		err := cluster.WithAffiliate(id, func(a *Affiliate) error {
			a.Data = []byte(id)
			return nil
		})
		if err != nil {
			t.Fatalf("WithAffiliate failed: %v", err)
		}
	}

	// List affiliates
	affiliates := cluster.ListAffiliates()

	if len(affiliates) != len(affiliateIDs) {
		t.Errorf("expected %d affiliates, got %d", len(affiliateIDs), len(affiliates))
	}

	// Verify all IDs are present
	foundIDs := make(map[string]bool)
	for _, a := range affiliates {
		foundIDs[a.ID] = true
	}

	for _, id := range affiliateIDs {
		if !foundIDs[id] {
			t.Errorf("affiliate %q not found in list", id)
		}
	}
}

func TestListAffiliatesReturnsSnapshot(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create affiliate
	err := cluster.WithAffiliate("test", func(a *Affiliate) error {
		a.Data = []byte("original")
		return nil
	})
	if err != nil {
		t.Fatalf("WithAffiliate failed: %v", err)
	}

	// Get snapshot
	affiliates1 := cluster.ListAffiliates()

	// Modify affiliate
	err = cluster.WithAffiliate("test", func(a *Affiliate) error {
		a.Data = []byte("modified")
		return nil
	})
	if err != nil {
		t.Fatalf("second WithAffiliate failed: %v", err)
	}

	// Original snapshot should be unchanged
	if string(affiliates1[0].Data) != "original" {
		t.Error("snapshot was modified")
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Subscribe
	snapshot, sub := cluster.Subscribe()

	if snapshot == nil {
		t.Error("snapshot is nil")
	}

	if sub == nil {
		t.Fatal("subscription is nil")
	}

	// Verify subscription was added
	cluster.subscriptionsMu.Lock()
	subCount := len(cluster.subscriptions)
	cluster.subscriptionsMu.Unlock()

	if subCount != 1 {
		t.Errorf("expected 1 subscription, got %d", subCount)
	}

	// Unsubscribe
	cluster.Unsubscribe(sub)

	// Verify subscription was removed
	cluster.subscriptionsMu.Lock()
	subCount = len(cluster.subscriptions)
	cluster.subscriptionsMu.Unlock()

	if subCount != 0 {
		t.Errorf("expected 0 subscriptions after unsubscribe, got %d", subCount)
	}
}

func TestSubscribeReturnsSnapshot(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create affiliates
	affiliateIDs := []string{"affiliate1", "affiliate2"}
	for _, id := range affiliateIDs {
		err := cluster.WithAffiliate(id, func(a *Affiliate) error {
			return nil
		})
		if err != nil {
			t.Fatalf("WithAffiliate failed: %v", err)
		}
	}

	// Subscribe and get snapshot
	snapshot, _ := cluster.Subscribe()

	if len(snapshot) != len(affiliateIDs) {
		t.Errorf("expected %d affiliates in snapshot, got %d", len(affiliateIDs), len(snapshot))
	}
}

func TestNotifyUpdate(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create affiliate
	err := cluster.WithAffiliate("test", func(a *Affiliate) error {
		a.Data = []byte("test")
		a.Changed = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithAffiliate failed: %v", err)
	}

	// Subscribe
	_, sub := cluster.Subscribe()
	defer cluster.Unsubscribe(sub)

	// Get affiliate
	cluster.affiliatesMu.Lock()
	affiliate := cluster.affiliates["test"]
	cluster.affiliatesMu.Unlock()

	// Notify
	go cluster.NotifyUpdate(affiliate)

	// Receive notification
	select {
	case notification := <-sub.Ch():
		if notification.Affiliate.ID != "test" {
			t.Errorf("expected affiliate ID 'test', got %q", notification.Affiliate.ID)
		}
		if notification.Deleted {
			t.Error("expected Deleted to be false")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for notification")
	}
}

func TestNotifyUpdateClearsChangedFlag(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create affiliate
	err := cluster.WithAffiliate("test", func(a *Affiliate) error {
		a.Changed = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithAffiliate failed: %v", err)
	}

	cluster.affiliatesMu.Lock()
	affiliate := cluster.affiliates["test"]
	cluster.affiliatesMu.Unlock()

	// Notify
	cluster.NotifyUpdate(affiliate)

	// Changed flag should be cleared
	if affiliate.Changed {
		t.Error("Changed flag was not cleared")
	}
}

func TestNotifyUpdateSkipsUnchanged(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Create affiliate without setting Changed flag
	err := cluster.WithAffiliate("test", func(a *Affiliate) error {
		a.Changed = false
		return nil
	})
	if err != nil {
		t.Fatalf("WithAffiliate failed: %v", err)
	}

	// Subscribe
	_, sub := cluster.Subscribe()
	defer cluster.Unsubscribe(sub)

	cluster.affiliatesMu.Lock()
	affiliate := cluster.affiliates["test"]
	cluster.affiliatesMu.Unlock()

	// Notify - should be skipped
	cluster.NotifyUpdate(affiliate)

	// Should not receive notification
	select {
	case <-sub.Ch():
		t.Error("received notification for unchanged affiliate")
	case <-time.After(50 * time.Millisecond):
		// Expected - no notification
	}
}

func TestClusterGarbageCollect(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	now := time.Now()
	expired := now.Add(-1 * time.Hour)
	active := now.Add(1 * time.Hour)

	// Add expired and active affiliates
	cluster.affiliatesMu.Lock()
	cluster.affiliates["expired"] = &Affiliate{
		ID:         "expired",
		Data:       []byte("test"),
		Expiration: expired,
	}
	cluster.affiliates["active"] = &Affiliate{
		ID:         "active",
		Data:       []byte("test"),
		Expiration: active,
	}
	cluster.affiliatesMu.Unlock()

	// Run GC
	shouldDelete := cluster.GarbageCollect(now)

	// Should not delete cluster (has active affiliate)
	if shouldDelete {
		t.Error("GarbageCollect indicated cluster should be deleted")
	}

	// Verify expired removed, active remains
	cluster.affiliatesMu.Lock()
	_, expiredExists := cluster.affiliates["expired"]
	_, activeExists := cluster.affiliates["active"]
	cluster.affiliatesMu.Unlock()

	if expiredExists {
		t.Error("expired affiliate was not removed")
	}
	if !activeExists {
		t.Error("active affiliate was removed")
	}
}

func TestClusterGarbageCollectEmpty(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Empty cluster with no subscriptions should be deleted
	shouldDelete := cluster.GarbageCollect(time.Now())

	if !shouldDelete {
		t.Error("empty cluster should be marked for deletion")
	}
}

func TestClusterGarbageCollectWithSubscriptions(t *testing.T) {
	logger := zap.NewNop()
	cluster := NewCluster("test-cluster", logger)

	// Add subscription
	_, sub := cluster.Subscribe()
	defer cluster.Unsubscribe(sub)

	// Cluster with subscriptions should not be deleted
	shouldDelete := cluster.GarbageCollect(time.Now())

	if shouldDelete {
		t.Error("cluster with subscriptions should not be marked for deletion")
	}
}
