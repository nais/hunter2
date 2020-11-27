package metrics

import "github.com/prometheus/client_golang/prometheus"

const namespace = "hunter2"

var (
	Success = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "successes",
			Namespace: namespace,
			Help:      "Cumulative number of successful operations"},
	)
	Errors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "errors",
			Namespace: namespace,
			Help:      "Cumulative number of failed operations"},
		[]string{"operation"},
	)
	GoogleSecretManagerResponseTime = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:      "secret_manager_response_time",
			Namespace: namespace,
			Help:      "Response time for calls to Google Secret Manager",
		})
)
