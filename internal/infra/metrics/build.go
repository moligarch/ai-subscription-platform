package metrics

import "github.com/prometheus/client_golang/prometheus"

func init() { 
	register(buildInfo) 
}

var buildInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "build_info",
		Help: "A constant metric with labels for version and commit hash.",
	},
	[]string{"version", "commit"},
)

func SetBuildInfo(version, commit string) {
	buildInfo.WithLabelValues(version, commit).Set(1)
}
