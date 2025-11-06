package state

import (
	"errors"
	"sync"
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
	"go.uber.org/zap"
)

var (
	// ErrAffiliateLimit is returned when the affiliate limit is reached
	ErrAffiliateLimit = errors.New("affiliate limit reached")
)

// Cluster represents a discovery cluster
type Cluster struct {
	ID string

	affiliatesMu  sync.Mutex
	affiliates    map[string]*Affiliate

	subscriptionsMu sync.Mutex
	subscriptions   []*Subscription

	logger *zap.Logger
}

// NewCluster creates a new cluster
func NewCluster(id string, logger *zap.Logger) *Cluster {
	return &Cluster{
		ID:            id,
		affiliates:    make(map[string]*Affiliate),
		subscriptions: make([]*Subscription, 0),
		logger:        logger,
	}
}

// WithAffiliate executes a function with the affiliate, creating it if it doesn't exist
func (c *Cluster) WithAffiliate(id string, fn func(*Affiliate) error) error {
	c.affiliatesMu.Lock()
	defer c.affiliatesMu.Unlock()

	affiliate, exists := c.affiliates[id]
	if !exists {
		if len(c.affiliates) >= limits.ClusterAffiliatesMax {
			c.logger.Warn("cluster affiliate limit reached",
				zap.String("cluster_id", c.ID),
				zap.Int("current_count", len(c.affiliates)),
				zap.Int("max", limits.ClusterAffiliatesMax),
			)
			return ErrAffiliateLimit
		}
		affiliate = &Affiliate{
			ID:        id,
			Endpoints: make([]EndpointEntry, 0),
		}
		c.affiliates[id] = affiliate
		c.logger.Debug("created new affiliate",
			zap.String("cluster_id", c.ID),
			zap.String("affiliate_id", id),
		)
	}

	return fn(affiliate)
}

// DeleteAffiliate removes an affiliate from the cluster
func (c *Cluster) DeleteAffiliate(id string) {
	c.affiliatesMu.Lock()
	affiliate, exists := c.affiliates[id]
	if exists {
		delete(c.affiliates, id)
	}
	c.affiliatesMu.Unlock()

	if exists {
		c.notify(Notification{
			Affiliate: affiliate,
			Deleted:   true,
		})
	}
}

// ListAffiliates returns a snapshot of all affiliates
func (c *Cluster) ListAffiliates() []*Affiliate {
	c.affiliatesMu.Lock()
	defer c.affiliatesMu.Unlock()

	affiliates := make([]*Affiliate, 0, len(c.affiliates))
	for _, a := range c.affiliates {
		// Create a copy to avoid race conditions
		affiliateCopy := *a
		affiliates = append(affiliates, &affiliateCopy)
	}

	return affiliates
}

// Subscribe creates a new subscription and returns a snapshot of affiliates
func (c *Cluster) Subscribe() ([]*Affiliate, *Subscription) {
	snapshot := c.ListAffiliates()

	sub := NewSubscription()

	c.subscriptionsMu.Lock()
	c.subscriptions = append(c.subscriptions, sub)
	c.subscriptionsMu.Unlock()

	return snapshot, sub
}

// Unsubscribe removes a subscription
func (c *Cluster) Unsubscribe(sub *Subscription) {
	c.subscriptionsMu.Lock()
	defer c.subscriptionsMu.Unlock()

	for i, s := range c.subscriptions {
		if s == sub {
			// Remove by swapping with last element
			c.subscriptions[i] = c.subscriptions[len(c.subscriptions)-1]
			c.subscriptions = c.subscriptions[:len(c.subscriptions)-1]
			break
		}
	}

	sub.Close()
}

// NotifyUpdate notifies subscribers of an affiliate update
func (c *Cluster) NotifyUpdate(affiliate *Affiliate) {
	if !affiliate.Changed {
		c.logger.Debug("skipping notification - affiliate not changed",
			zap.String("cluster_id", c.ID),
			zap.String("affiliate_id", affiliate.ID),
		)
		return
	}

	affiliate.Changed = false

	// Create a copy for notification
	affiliateCopy := *affiliate

	c.logger.Debug("notifying subscribers of affiliate update",
		zap.String("cluster_id", c.ID),
		zap.String("affiliate_id", affiliate.ID),
		zap.Int("subscriber_count", len(c.subscriptions)),
	)

	c.notify(Notification{
		Affiliate: &affiliateCopy,
		Deleted:   false,
	})
}

// notify sends a notification to all subscribers
func (c *Cluster) notify(notification Notification) {
	c.subscriptionsMu.Lock()
	// Clone subscriptions to avoid holding lock during send
	subs := make([]*Subscription, len(c.subscriptions))
	copy(subs, c.subscriptions)
	c.subscriptionsMu.Unlock()

	for _, sub := range subs {
		sub.Send(notification)
	}
}

// GarbageCollect removes expired affiliates and endpoints
// Returns true if the cluster is empty and should be removed
func (c *Cluster) GarbageCollect(now time.Time) bool {
	c.affiliatesMu.Lock()
	defer c.affiliatesMu.Unlock()

	toDelete := make([]string, 0)

	for id, affiliate := range c.affiliates {
		if affiliate.GarbageCollect(now) {
			toDelete = append(toDelete, id)
		}
	}

	// Delete expired affiliates
	for _, id := range toDelete {
		affiliate := c.affiliates[id]
		delete(c.affiliates, id)

		// Notify after releasing lock
		go c.notify(Notification{
			Affiliate: affiliate,
			Deleted:   true,
		})
	}

	// Cluster can be removed if it has no affiliates and no subscriptions
	c.subscriptionsMu.Lock()
	hasSubscriptions := len(c.subscriptions) > 0
	c.subscriptionsMu.Unlock()

	return len(c.affiliates) == 0 && !hasSubscriptions
}
