package limits

import "time"

const (
	// ClusterAffiliatesMax is the maximum number of affiliates per cluster
	ClusterAffiliatesMax = 100

	// AffiliateEndpointsMax is the maximum number of endpoints per affiliate
	AffiliateEndpointsMax = 20

	// IPRateRequestsPerSecondMax is the maximum requests per second per IP
	IPRateRequestsPerSecondMax = 15

	// IPRateBurstSizeMax is the maximum burst size per IP
	IPRateBurstSizeMax = 60

	// IPRateGarbageCollectionPeriod is how often to clean up rate limiters
	IPRateGarbageCollectionPeriod = time.Minute
)
