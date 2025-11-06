package state

import (
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
)

// EndpointEntry represents an endpoint with expiration
type EndpointEntry struct {
	Data       []byte
	Expiration time.Time
}

// Affiliate represents a cluster member with encrypted data
type Affiliate struct {
	ID         string
	Data       []byte
	Expiration time.Time
	Endpoints  []EndpointEntry
	Changed    bool
}

// Update updates the affiliate data and expiration
func (a *Affiliate) Update(data []byte, expiration time.Time) {
	a.Data = data
	a.Expiration = expiration
	a.Changed = true
}

// MergeEndpoints merges new endpoints into the affiliate
func (a *Affiliate) MergeEndpoints(endpoints []EndpointEntry) {
	// Create a map for quick lookup of existing endpoints
	existingMap := make(map[string]*EndpointEntry)
	for i := range a.Endpoints {
		key := string(a.Endpoints[i].Data)
		existingMap[key] = &a.Endpoints[i]
	}

	// Update or add endpoints
	for _, ep := range endpoints {
		key := string(ep.Data)
		if existing, ok := existingMap[key]; ok {
			// Update expiration if newer
			if ep.Expiration.After(existing.Expiration) {
				existing.Expiration = ep.Expiration
				a.Changed = true
			}
		} else if len(a.Endpoints) < limits.AffiliateEndpointsMax {
			// Add new endpoint
			a.Endpoints = append(a.Endpoints, ep)
			a.Changed = true
		}
	}
}

// GarbageCollect removes expired data and endpoints
// Returns true if the affiliate should be deleted
func (a *Affiliate) GarbageCollect(now time.Time) bool {
	// Check if affiliate itself is expired
	if now.After(a.Expiration) {
		return true
	}

	// Remove expired endpoints
	newEndpoints := make([]EndpointEntry, 0, len(a.Endpoints))
	for _, ep := range a.Endpoints {
		if now.Before(ep.Expiration) {
			newEndpoints = append(newEndpoints, ep)
		}
	}

	if len(newEndpoints) != len(a.Endpoints) {
		a.Endpoints = newEndpoints
		a.Changed = true
	}

	return false
}
