package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ScheduleLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "galaxy_schedule_latency",
			Help:    "Galaxy schedule latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 7),
		}, []string{"func"})

	CloudProviderLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "galaxy_cloud_provider_latency",
			Help:    "Galaxy cloud provider latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 7),
		}, []string{"func"})
)

// MustRegister registers all metrics
func MustRegister() {
	prometheus.MustRegister(ScheduleLatency, CloudProviderLatency)
}
