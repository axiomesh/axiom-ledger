package precheck

import "github.com/prometheus/client_golang/prometheus"

var (
	basicCheckDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "axiom_ledger",
			Subsystem: "pre_check",
			Name:      "basic_check_duration_seconds",
			Help:      "The average latency of basic check",
		},
		[]string{"type"},
	)
	verifySignatureDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "axiom_ledger",
			Subsystem: "pre_check",
			Name:      "verify_signature_duration_seconds",
			Help:      "The average latency of verify signature",
		},
		[]string{"type"},
	)
	verifyBalanceDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "axiom_ledger",
			Subsystem: "pre_check",
			Name:      "verify_balance_duration_seconds",
			Help:      "The average latency of verify balance",
		},
		[]string{"type"},
	)
	rejectTxCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "axiom_ledger",
			Subsystem: "pre_check",
			Name:      "reject_tx_counter",
			Help:      "The number of rejected tx",
		},
		[]string{"reason"},
	)
	validTxCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "axiom_ledger",
			Subsystem: "pre_check",
			Name:      "valid_tx_counter",
			Help:      "The number of valid tx",
		},
	)
)

func init() {
	prometheus.MustRegister(basicCheckDuration)
	prometheus.MustRegister(verifySignatureDuration)
	prometheus.MustRegister(verifyBalanceDuration)
	prometheus.MustRegister(rejectTxCounter)
	prometheus.MustRegister(validTxCounter)
}
