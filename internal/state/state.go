package state

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// State manages all clusters
type State struct {
	mu       sync.RWMutex
	clusters map[string]*Cluster

	logger *zap.Logger

	// Metrics
	gcRuns               prometheus.Counter
	gcCollectedClusters  prometheus.Counter
	gcCollectedAffiliates prometheus.Counter
}

// NewState creates a new state manager
func NewState(logger *zap.Logger) *State {
	return &State{
		clusters: make(map[string]*Cluster),
		logger:   logger,
		gcRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "discovery_gc_runs_total",
			Help: "Total number of garbage collection runs",
		}),
		gcCollectedClusters: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "discovery_gc_collected_clusters_total",
			Help: "Total number of clusters collected by GC",
		}),
		gcCollectedAffiliates: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "discovery_gc_collected_affiliates_total",
			Help: "Total number of affiliates collected by GC",
		}),
	}
}

// GetCluster returns a cluster by ID, creating it if it doesn't exist
func (s *State) GetCluster(id string) *Cluster {
	s.mu.RLock()
	cluster, exists := s.clusters[id]
	s.mu.RUnlock()

	if exists {
		return cluster
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	cluster, exists = s.clusters[id]
	if exists {
		return cluster
	}

	cluster = NewCluster(id, s.logger)
	s.clusters[id] = cluster

	s.logger.Debug("created new cluster",
		zap.String("cluster_id", id),
	)

	return cluster
}

// GarbageCollect removes expired affiliates and empty clusters
func (s *State) GarbageCollect(now time.Time) {
	s.gcRuns.Inc()

	s.mu.Lock()
	defer s.mu.Unlock()

	toDelete := make([]string, 0)
	affiliatesCollected := 0

	for id, cluster := range s.clusters {
		// Count affiliates before GC
		beforeCount := len(cluster.affiliates)

		if cluster.GarbageCollect(now) {
			toDelete = append(toDelete, id)
		}

		// Count affiliates after GC
		afterCount := len(cluster.affiliates)
		affiliatesCollected += (beforeCount - afterCount)
	}

	// Remove empty clusters
	for _, id := range toDelete {
		delete(s.clusters, id)
	}

	if len(toDelete) > 0 || affiliatesCollected > 0 {
		s.logger.Info("garbage collection completed",
			zap.Int("clusters_collected", len(toDelete)),
			zap.Int("affiliates_collected", affiliatesCollected),
			zap.Int("clusters_remaining", len(s.clusters)),
		)
	}

	s.gcCollectedClusters.Add(float64(len(toDelete)))
	s.gcCollectedAffiliates.Add(float64(affiliatesCollected))
}

// RunGC runs garbage collection periodically
func (s *State) RunGC(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.GarbageCollect(now)
		}
	}
}

// Describe implements prometheus.Collector
func (s *State) Describe(ch chan<- *prometheus.Desc) {
	s.gcRuns.Describe(ch)
	s.gcCollectedClusters.Describe(ch)
	s.gcCollectedAffiliates.Describe(ch)

	// Describe gauges
	ch <- prometheus.NewDesc(
		"discovery_clusters_total",
		"Total number of clusters",
		nil, nil,
	)
	ch <- prometheus.NewDesc(
		"discovery_affiliates_total",
		"Total number of affiliates across all clusters",
		nil, nil,
	)
	ch <- prometheus.NewDesc(
		"discovery_endpoints_total",
		"Total number of endpoints across all affiliates",
		nil, nil,
	)
	ch <- prometheus.NewDesc(
		"discovery_subscriptions_total",
		"Total number of active subscriptions",
		nil, nil,
	)
}

// Collect implements prometheus.Collector
func (s *State) Collect(ch chan<- prometheus.Metric) {
	s.gcRuns.Collect(ch)
	s.gcCollectedClusters.Collect(ch)
	s.gcCollectedAffiliates.Collect(ch)

	// Collect current stats
	s.mu.RLock()
	clusterCount := len(s.clusters)
	affiliateCount := 0
	endpointCount := 0
	subscriptionCount := 0

	for _, cluster := range s.clusters {
		cluster.affiliatesMu.Lock()
		affiliateCount += len(cluster.affiliates)
		for _, affiliate := range cluster.affiliates {
			endpointCount += len(affiliate.Endpoints)
		}
		cluster.affiliatesMu.Unlock()

		cluster.subscriptionsMu.Lock()
		subscriptionCount += len(cluster.subscriptions)
		cluster.subscriptionsMu.Unlock()
	}
	s.mu.RUnlock()

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("discovery_clusters_total", "Total number of clusters", nil, nil),
		prometheus.GaugeValue,
		float64(clusterCount),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("discovery_affiliates_total", "Total number of affiliates", nil, nil),
		prometheus.GaugeValue,
		float64(affiliateCount),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("discovery_endpoints_total", "Total number of endpoints", nil, nil),
		prometheus.GaugeValue,
		float64(endpointCount),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("discovery_subscriptions_total", "Total number of subscriptions", nil, nil),
		prometheus.GaugeValue,
		float64(subscriptionCount),
	)
}
