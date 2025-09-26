package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() {
	register(
		usersRegisteredTotal,
		telegramCommandsReceivedTotal,
		telegramRateLimitTriggeredTotal,
	)
}

var (
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

	telegramRateLimitTriggeredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "telegram_rate_limit_triggered_total",
			Help: "Total number of times users have been rate-limited.",
		},
	)
)

func IncUsersRegistered() {
	usersRegisteredTotal.Inc()
}

func IncTelegramCommand(command string) {
	telegramCommandsReceivedTotal.WithLabelValues(norm(command)).Inc()
}

func IncRateLimitTriggered() {
	telegramRateLimitTriggeredTotal.Inc()
}
