package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() { 
	register(
		PaymentVerifyRequests, 
		PaymentVerifyDuration, 
		PaymentDMTotal,
		) 
}

var (
	// Count of verify calls grouped by result and bounded reason.
	// result: ok|fail
	// reason (fail only): bad_json|missing_authority|not_ok_status|confirm_error|method_not_allowed|unknown
	PaymentVerifyRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_verify_requests_total",
			Help: "Count of /api/v1/payment/verify calls by result and reason.",
		},
		[]string{"result", "reason"},
	)

	// Latency of verify handler grouped by result.
	PaymentVerifyDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payment_verify_duration_seconds",
			Help:    "Duration of /api/v1/payment/verify handler in seconds.",
			Buckets: []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		},
		[]string{"result"},
	)

	// Telegram DM attempts grouped by kind and status.
	// kind: success|failure
	// status: sent|error|no_user
	PaymentDMTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_dm_total",
			Help: "Telegram DMs about payment status by kind and delivery status.",
		},
		[]string{"kind", "status"},
	)
)