// File: internal/infra/metrics/metrics.go
package metrics

import (
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	aiTokensIn = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_tokens_in",
			Help: "Sum of prompt (input) tokens per provider/model.",
		},
		[]string{"provider", "model"},
	)

	aiTokensOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_tokens_out",
			Help: "Sum of completion (output) tokens per provider/model.",
		},
		[]string{"provider", "model"},
	)

	aiTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_tokens_total",
			Help: "Sum of total tokens per provider/model.",
		},
		[]string{"provider", "model"},
	)

	aiCostMicro = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_cost_micro",
			Help: "Total micro-credits spent per provider/model.",
		},
		[]string{"provider", "model"},
	)

	aiCallsLatencyMs = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_calls_latency_ms",
			Help:    "AI call latency distribution in milliseconds.",
			Buckets: []float64{10, 25, 50, 100, 200, 400, 800, 1600, 3000, 5000},
		},
		[]string{"provider", "model", "success"},
	)

	aiPrecheckBlocks = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_precheck_blocks",
			Help: "Count of pre-send affordability blocks per provider/model.",
		},
		[]string{"provider", "model"},
	)

	paymentsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payments_total",
			Help: "Payments by status (initiated/succeeded/failed).",
		},
		[]string{"status"},
	)
)

// MustRegister registers collectors with the default registry (idempotent).
func MustRegister() {
	once.Do(func() {
		prometheus.MustRegister(
			aiTokensIn, aiTokensOut, aiTokensTotal,
			aiCostMicro, aiCallsLatencyMs, aiPrecheckBlocks,
			paymentsTotal,
		)
	})
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// -------- Chat helpers --------

func PrecheckBlocked(provider, model string) {
	aiPrecheckBlocks.WithLabelValues(norm(provider), norm(model)).Inc()
}

func ObserveChatUsage(provider, model string, tokensIn, tokensOut, tokensTotal int, costMicro int64, latencyMs int, success bool) {
	lbl := []string{norm(provider), norm(model)}
	aiTokensIn.WithLabelValues(lbl...).Add(float64(tokensIn))
	aiTokensOut.WithLabelValues(lbl...).Add(float64(tokensOut))
	aiTokensTotal.WithLabelValues(lbl...).Add(float64(tokensTotal))
	aiCostMicro.WithLabelValues(lbl...).Add(float64(costMicro))
	aiCallsLatencyMs.WithLabelValues(norm(provider), norm(model), strconv.FormatBool(success)).
		Observe(float64(latencyMs))
}

// -------- Payment helpers --------

func IncPayment(status string) {
	paymentsTotal.WithLabelValues(norm(status)).Inc()
}
