package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() { register(aiJobsProcessedTotal) }

var aiJobsProcessedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "ai_jobs_processed_total",
		Help: "Total number of AI jobs processed, labeled by status.",
	},
	[]string{"status"}, // 'completed', 'failed'
)

func IncAIJob(status string) {
	aiJobsProcessedTotal.WithLabelValues(norm(status)).Inc()
}
