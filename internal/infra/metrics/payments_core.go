package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() {
	register(
		paymentsTotal,
		paymentsRevenueTotal,
	)
}

var (
	paymentsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payments_total",
			Help: "Payments by status (initiated/succeeded/failed).",
		},
		[]string{"status"},
	)

	paymentsRevenueTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payments_revenue_total",
			Help: "The total monetary value of successful payments, labeled by currency.",
		},
		[]string{"currency"},
	)
)

func IncPayment(status string) {
	paymentsTotal.WithLabelValues(norm(status)).Inc()
}

func AddPaymentRevenue(currency string, amount int64) {
	paymentsRevenueTotal.WithLabelValues(norm(currency)).Add(float64(amount))
}
