package metrics

import "github.com/prometheus/client_golang/prometheus"

const (
	namespace      = "hunter2"
	LabelStatus    = "status"
	LabelSystem    = "system"
	LabelOperation = "operation"
)

type Status = string

type Operation = string

type System = string

const (
	StatusSuccess     Status = "success"
	StatusError       Status = "error"
	StatusNotManaged  Status = "not_managed"
	StatusInvalidData Status = "invalid_data"
	StatusNoSyncLabel Status = "no_sync_label"

	SystemKubernetes    System = "kubernetes"
	SystemPubSub        System = "pubsub"
	SystemSecretManager System = "secret_manager"

	OperationCreate Operation = "create"
	OperationRead   Operation = "read"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

// Zero out all possible label combinations
func InitLabels() {
	statuses := []Status{StatusSuccess, StatusError, StatusNotManaged, StatusInvalidData, StatusNoSyncLabel}
	systems := []System{SystemKubernetes, SystemPubSub, SystemSecretManager}
	operations := []Operation{OperationCreate, OperationRead, OperationUpdate, OperationDelete}

	for _, status := range statuses {
		for _, system := range systems {
			for _, operation := range operations {
				Requests.With(prometheus.Labels{
					LabelOperation: operation,
					LabelStatus:    status,
					LabelSystem:    system,
				})
			}
		}
	}
}

func ErrorStatus(err error, fallback Status) Status {
	if err == nil {
		return StatusSuccess
	}
	return fallback
}

func LogRequest(system System, operation Operation, status Status) {
	Requests.With(prometheus.Labels{
		LabelOperation: operation,
		LabelStatus:    status,
		LabelSystem:    system,
	}).Inc()
}

var (
	Requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "requests",
			Namespace: namespace,
			Help:      "Cumulative number of incoming requests",
		},
		[]string{
			LabelOperation,
			LabelStatus,
			LabelSystem,
		},
	)
	GoogleSecretManagerResponseTime = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:      "secret_manager_response_time",
			Namespace: namespace,
			Help:      "Response time for calls to Google Secret Manager",
		})
)
