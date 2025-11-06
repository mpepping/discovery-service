package state

import (
	"testing"
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
)

func TestAffiliateUpdate(t *testing.T) {
	affiliate := &Affiliate{
		ID: "test",
	}

	data := []byte("test-data")
	expiration := time.Now().Add(1 * time.Hour)

	affiliate.Update(data, expiration)

	if string(affiliate.Data) != string(data) {
		t.Errorf("expected data %q, got %q", string(data), string(affiliate.Data))
	}

	if !affiliate.Expiration.Equal(expiration) {
		t.Errorf("expected expiration %v, got %v", expiration, affiliate.Expiration)
	}

	if !affiliate.Changed {
		t.Error("Changed flag not set")
	}
}

func TestMergeEndpointsNew(t *testing.T) {
	affiliate := &Affiliate{
		ID:        "test",
		Endpoints: make([]EndpointEntry, 0),
	}

	expiration := time.Now().Add(1 * time.Hour)
	newEndpoints := []EndpointEntry{
		{Data: []byte("endpoint1"), Expiration: expiration},
		{Data: []byte("endpoint2"), Expiration: expiration},
	}

	affiliate.MergeEndpoints(newEndpoints)

	if len(affiliate.Endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(affiliate.Endpoints))
	}

	if !affiliate.Changed {
		t.Error("Changed flag not set")
	}
}

func TestMergeEndpointsUpdate(t *testing.T) {
	oldExpiration := time.Now().Add(1 * time.Hour)
	newExpiration := time.Now().Add(2 * time.Hour)

	affiliate := &Affiliate{
		ID: "test",
		Endpoints: []EndpointEntry{
			{Data: []byte("endpoint1"), Expiration: oldExpiration},
		},
		Changed: false,
	}

	// Merge with newer expiration for same endpoint
	newEndpoints := []EndpointEntry{
		{Data: []byte("endpoint1"), Expiration: newExpiration},
	}

	affiliate.MergeEndpoints(newEndpoints)

	if len(affiliate.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(affiliate.Endpoints))
	}

	// Should update to newer expiration
	if !affiliate.Endpoints[0].Expiration.Equal(newExpiration) {
		t.Errorf("expected expiration %v, got %v", newExpiration, affiliate.Endpoints[0].Expiration)
	}

	if !affiliate.Changed {
		t.Error("Changed flag not set")
	}
}

func TestMergeEndpointsOlderExpiration(t *testing.T) {
	newExpiration := time.Now().Add(2 * time.Hour)
	oldExpiration := time.Now().Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID: "test",
		Endpoints: []EndpointEntry{
			{Data: []byte("endpoint1"), Expiration: newExpiration},
		},
		Changed: false,
	}

	// Merge with older expiration for same endpoint
	endpoints := []EndpointEntry{
		{Data: []byte("endpoint1"), Expiration: oldExpiration},
	}

	affiliate.MergeEndpoints(endpoints)

	// Should not update to older expiration
	if !affiliate.Endpoints[0].Expiration.Equal(newExpiration) {
		t.Errorf("expiration was changed to older value")
	}

	if affiliate.Changed {
		t.Error("Changed flag should not be set when expiration is older")
	}
}

func TestMergeEndpointsLimit(t *testing.T) {
	affiliate := &Affiliate{
		ID:        "test",
		Endpoints: make([]EndpointEntry, 0),
	}

	expiration := time.Now().Add(1 * time.Hour)

	// Fill up to limit
	endpoints := make([]EndpointEntry, limits.AffiliateEndpointsMax)
	for i := 0; i < limits.AffiliateEndpointsMax; i++ {
		endpoints[i] = EndpointEntry{
			Data:       []byte{byte(i)},
			Expiration: expiration,
		}
	}

	affiliate.MergeEndpoints(endpoints)

	if len(affiliate.Endpoints) != limits.AffiliateEndpointsMax {
		t.Errorf("expected %d endpoints, got %d", limits.AffiliateEndpointsMax, len(affiliate.Endpoints))
	}

	// Try to add one more
	affiliate.Changed = false
	moreEndpoints := []EndpointEntry{
		{Data: []byte("overflow"), Expiration: expiration},
	}

	affiliate.MergeEndpoints(moreEndpoints)

	// Should still be at limit
	if len(affiliate.Endpoints) != limits.AffiliateEndpointsMax {
		t.Errorf("expected %d endpoints after overflow, got %d", limits.AffiliateEndpointsMax, len(affiliate.Endpoints))
	}

	// Changed should not be set since nothing was added
	if affiliate.Changed {
		t.Error("Changed flag should not be set when at limit")
	}
}

func TestMergeEndpointsMixed(t *testing.T) {
	expiration := time.Now().Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID: "test",
		Endpoints: []EndpointEntry{
			{Data: []byte("existing"), Expiration: expiration},
		},
	}

	// Merge new and existing endpoints
	newEndpoints := []EndpointEntry{
		{Data: []byte("existing"), Expiration: expiration.Add(1 * time.Hour)}, // Update
		{Data: []byte("new"), Expiration: expiration},                         // Add
	}

	affiliate.MergeEndpoints(newEndpoints)

	if len(affiliate.Endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(affiliate.Endpoints))
	}

	if !affiliate.Changed {
		t.Error("Changed flag not set")
	}
}

func TestAffiliateGarbageCollectExpired(t *testing.T) {
	expiredTime := time.Now().Add(-1 * time.Hour)

	affiliate := &Affiliate{
		ID:         "test",
		Data:       []byte("test"),
		Expiration: expiredTime,
	}

	shouldDelete := affiliate.GarbageCollect(time.Now())

	if !shouldDelete {
		t.Error("expired affiliate should be marked for deletion")
	}
}

func TestAffiliateGarbageCollectActive(t *testing.T) {
	futureTime := time.Now().Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID:         "test",
		Data:       []byte("test"),
		Expiration: futureTime,
	}

	shouldDelete := affiliate.GarbageCollect(time.Now())

	if shouldDelete {
		t.Error("active affiliate should not be marked for deletion")
	}
}

func TestAffiliateGarbageCollectExpiredEndpoints(t *testing.T) {
	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID:         "test",
		Data:       []byte("test"),
		Expiration: futureTime,
		Endpoints: []EndpointEntry{
			{Data: []byte("expired"), Expiration: expiredTime},
			{Data: []byte("active"), Expiration: futureTime},
		},
		Changed: false,
	}

	shouldDelete := affiliate.GarbageCollect(now)

	if shouldDelete {
		t.Error("affiliate with active endpoints should not be deleted")
	}

	if len(affiliate.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint after GC, got %d", len(affiliate.Endpoints))
	}

	if string(affiliate.Endpoints[0].Data) != "active" {
		t.Errorf("expected 'active' endpoint to remain, got %q", string(affiliate.Endpoints[0].Data))
	}

	if !affiliate.Changed {
		t.Error("Changed flag should be set when endpoints are removed")
	}
}

func TestAffiliateGarbageCollectNoExpiredEndpoints(t *testing.T) {
	now := time.Now()
	futureTime := now.Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID:         "test",
		Data:       []byte("test"),
		Expiration: futureTime,
		Endpoints: []EndpointEntry{
			{Data: []byte("endpoint1"), Expiration: futureTime},
			{Data: []byte("endpoint2"), Expiration: futureTime},
		},
		Changed: false,
	}

	shouldDelete := affiliate.GarbageCollect(now)

	if shouldDelete {
		t.Error("affiliate should not be deleted")
	}

	if len(affiliate.Endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(affiliate.Endpoints))
	}

	if affiliate.Changed {
		t.Error("Changed flag should not be set when nothing is removed")
	}
}

func TestAffiliateGarbageCollectAllEndpointsExpired(t *testing.T) {
	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	affiliate := &Affiliate{
		ID:         "test",
		Data:       []byte("test"),
		Expiration: futureTime,
		Endpoints: []EndpointEntry{
			{Data: []byte("expired1"), Expiration: expiredTime},
			{Data: []byte("expired2"), Expiration: expiredTime},
		},
		Changed: false,
	}

	shouldDelete := affiliate.GarbageCollect(now)

	if shouldDelete {
		t.Error("affiliate should not be deleted (only endpoints expired)")
	}

	if len(affiliate.Endpoints) != 0 {
		t.Errorf("expected 0 endpoints after GC, got %d", len(affiliate.Endpoints))
	}

	if !affiliate.Changed {
		t.Error("Changed flag should be set when all endpoints are removed")
	}
}
