package metrics

import (
	"telegram-ai-subscription/internal/domain/model"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	register(
		subscriptionsExpiredTotal,
		subscriptionsTotal,
	)
}

var (
	subscriptionsExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "subscriptions_expired_total",
			Help: "Total number of subscriptions processed by the expiry worker.",
		},
	)

	subscriptionsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subscriptions_total",
			Help: "Current number of subscriptions by status.",
		},
		[]string{"status"}, // 'active', 'reserved', 'finished', 'cancelled'
	)
)

func IncSubscriptionsExpired(count int) {
	subscriptionsExpiredTotal.Add(float64(count))
}

func SetSubscriptionsTotal(counts map[model.SubscriptionStatus]int) {
	// Set the gauge for each status present in the map.
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
