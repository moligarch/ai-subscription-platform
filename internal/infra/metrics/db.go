package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() { register(dbPoolStats) }

var dbPoolStats = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "db_pool_stats",
		Help: "Current state of the database connection pool.",
	},
	[]string{"state"}, // 'total', 'idle', 'in_use'
)

func SetDBPoolStats(total, idle, inUse int32) {
	dbPoolStats.WithLabelValues("total").Set(float64(total))
	dbPoolStats.WithLabelValues("idle").Set(float64(idle))
	dbPoolStats.WithLabelValues("in_use").Set(float64(inUse))
}
