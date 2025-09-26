package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once       sync.Once
	collectors []prometheus.Collector
)

// register is called by init() in each metrics file to enqueue collectors.
func register(cs ...prometheus.Collector) {
	collectors = append(collectors, cs...)
}

// MustRegister registers ALL enqueued collectors with Prometheus exactly once.
func MustRegister() {
	once.Do(func() {
		if len(collectors) > 0 {
			prometheus.MustRegister(collectors...)
		}
	})
}
