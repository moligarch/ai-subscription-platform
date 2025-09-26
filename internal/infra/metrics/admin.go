// File: internal/infra/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var adminCommandTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "admin_command_total",
		Help: "Tracks attempts to use admin commands.",
	},
	[]string{"command", "status"}, // status: 'authorized', 'unauthorized'
)

func IncAdminCommand(command, status string) {
	adminCommandTotal.WithLabelValues(norm(command), norm(status)).Inc()
}
