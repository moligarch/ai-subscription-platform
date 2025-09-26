package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() { register(cacheRequestsTotal) }

var cacheRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "cache_requests_total",
		Help: "Tracks cache hits and misses for various caches.",
	},
	[]string{"cache", "result"}, // e.g., cache="plan", result="hit"
)

func IncCacheRequest(cacheName, result string) {
	cacheRequestsTotal.WithLabelValues(norm(cacheName), norm(result)).Inc()
}
