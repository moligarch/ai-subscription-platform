// File: internal/infra/metrics/metrics.go
package metrics

import (
	"strconv"
	"strings"
	"sync"
	"telegram-ai-subscription/internal/domain/model"

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

	subscriptionsExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "subscriptions_expired_total",
			Help: "Total number of subscriptions processed by the expiry worker.",
		},
	)

	aiJobsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_jobs_processed_total",
			Help: "Total number of AI jobs processed, labeled by status.",
		},
		[]string{"status"}, // 'completed', 'failed'
	)

	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "A constant metric with labels for version and commit hash.",
		},
		[]string{"version", "commit"},
	)

	usersRegisteredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "users_registered_total",
			Help: "Total number of new users registered.",
		},
	)

	telegramCommandsReceivedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "telegram_commands_received_total",
			Help: "Counts incoming messages and commands from users.",
		},
		[]string{"command"},
	)

	dbPoolStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "db_pool_stats",
			Help: "Current state of the database connection pool.",
		},
		[]string{"state"}, // e.g., 'total', 'idle', 'in_use'
	)

	subscriptionsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subscriptions_total",
			Help: "Current number of subscriptions by status.",
		},
		[]string{"status"}, // 'active', 'reserved', etc.
	)

	paymentsRevenueTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payments_revenue_total",
			Help: "The total monetary value of successful payments, labeled by currency.",
		},
		[]string{"currency"},
	)

	telegramRateLimitTriggeredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "telegram_rate_limit_triggered_total",
			Help: "Total number of times users have been rate-limited.",
		},
	)

	cacheRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_requests_total",
			Help: "Tracks cache hits and misses for various caches.",
		},
		[]string{"cache", "result"}, // e.g., cache="plan", result="hit"
	)

	adminCommandTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "admin_command_total",
			Help: "Tracks attempts to use admin commands.",
		},
		[]string{"command", "status"}, // status: 'authorized', 'unauthorized'
	)
)

// MustRegister registers collectors with the default registry (idempotent).
func MustRegister() {
	once.Do(func() {
		prometheus.MustRegister(
			aiTokensIn, aiTokensOut, aiTokensTotal,
			aiCostMicro, aiCallsLatencyMs, aiPrecheckBlocks,
			paymentsTotal,
			subscriptionsExpiredTotal,
			aiJobsProcessedTotal,
			buildInfo,
			usersRegisteredTotal,
			telegramCommandsReceivedTotal,
			dbPoolStats,
			subscriptionsTotal,
			paymentsRevenueTotal,
			telegramRateLimitTriggeredTotal,
			cacheRequestsTotal,
			adminCommandTotal,
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

func IncSubscriptionsExpired(count int) {
	subscriptionsExpiredTotal.Add(float64(count))
}

func IncAIJob(status string) {
	aiJobsProcessedTotal.WithLabelValues(norm(status)).Inc()
}

func SetBuildInfo(version, commit string) {
	buildInfo.WithLabelValues(version, commit).Set(1)
}

func IncUsersRegistered() {
	usersRegisteredTotal.Inc()
}

func IncTelegramCommand(command string) {
	telegramCommandsReceivedTotal.WithLabelValues(norm(command)).Inc()
}

func SetDBPoolStats(total, idle, inUse int32) {
	dbPoolStats.WithLabelValues("total").Set(float64(total))
	dbPoolStats.WithLabelValues("idle").Set(float64(idle))
	dbPoolStats.WithLabelValues("in_use").Set(float64(inUse))
}

func SetSubscriptionsTotal(counts map[model.SubscriptionStatus]int) {
	// Set the gauge for each status. If a status doesn't exist in the map, it defaults to 0.
	statuses := []model.SubscriptionStatus{
		model.SubscriptionStatusActive,
		model.SubscriptionStatusReserved,
		model.SubscriptionStatusFinished,
		model.SubscriptionStatusCancelled,
	}
	for _, status := range statuses {
		if count, ok := counts[status]; ok {
			subscriptionsTotal.WithLabelValues(string(status)).Set(float64(count))
		}
	}
}

func AddPaymentRevenue(currency string, amount int64) {
	paymentsRevenueTotal.WithLabelValues(norm(currency)).Add(float64(amount))
}

func IncRateLimitTriggered() {
	telegramRateLimitTriggeredTotal.Inc()
}

func IncCacheRequest(cacheName, result string) {
	cacheRequestsTotal.WithLabelValues(norm(cacheName), norm(result)).Inc()
}

func IncAdminCommand(command, status string) {
	adminCommandTotal.WithLabelValues(norm(command), norm(status)).Inc()
}
