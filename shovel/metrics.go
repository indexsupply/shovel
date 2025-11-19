package shovel

import (
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ConsensusAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_consensus_attempts_total",
		Help: "Number of consensus evaluations per block range",
	}, []string{"src_name", "ig_name"})

	ConsensusFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_consensus_failures_total",
		Help: "Consensus attempts that failed to reach threshold",
	}, []string{"src_name", "ig_name"})

	ConsensusDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "shovel_consensus_duration_seconds",
		Help:    "Time taken to reach consensus for a block",
		Buckets: prometheus.DefBuckets,
	}, []string{"src_name", "ig_name"})

	ProviderErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_provider_error_total",
		Help: "RPC errors per provider",
	}, []string{"provider"})

	ConsensusExpansions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_consensus_expansions_total",
		Help: "Number of times provider pool was expanded during consensus retries",
	}, []string{"src_name", "ig_name"})

	ReceiptMismatch = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_receipt_mismatch_total",
		Help: "Number of receipt validation mismatches",
	}, []string{"src_name", "ig_name"})

	AuditVerifications = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_audit_verifications_total",
		Help: "Total confirmation audits executed",
	}, []string{"src_name", "ig_name"})

	AuditFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_audit_failures_total",
		Help: "Confirmation audits that failed verification",
	}, []string{"src_name", "ig_name"})

	AuditQueueLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shovel_audit_queue_length",
		Help: "Number of confirmed blocks pending audit",
	}, []string{"src_name"})

	AuditBacklogAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shovel_audit_backlog_age_seconds",
		Help: "Age in seconds of the oldest block pending audit for this source",
	}, []string{"src_name"})

	RepairRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_repair_requests_total",
		Help: "Total number of repair requests received",
	}, []string{"src_name", "ig_name"})

	RepairBlocksReprocessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shovel_repair_blocks_reprocessed_total",
		Help: "Total number of blocks reprocessed by repair jobs",
	}, []string{"src_name", "ig_name"})
)

type Metrics struct {
	start time.Time
	src   string
	ig    string
	once  sync.Once
}

func NewMetrics(src, ig string) *Metrics {
	return &Metrics{src: src, ig: ig}
}

func (m *Metrics) Start() {
	m.start = time.Now()
	ConsensusAttempts.WithLabelValues(m.src, m.ig).Inc()
}

func (m *Metrics) Failure() {
	ConsensusFailures.WithLabelValues(m.src, m.ig).Inc()
}

func (m *Metrics) ProviderError(p string) {
	ProviderErrors.WithLabelValues(p).Inc()
	slog.Warn("provider error", "p", p)
}

func (m *Metrics) Expansion() {
	ConsensusExpansions.WithLabelValues(m.src, m.ig).Inc()
}

func (m *Metrics) ReceiptMismatch() {
	ReceiptMismatch.WithLabelValues(m.src, m.ig).Inc()
}

func (m *Metrics) Stop() {
	m.once.Do(func() {
		ConsensusDuration.WithLabelValues(m.src, m.ig).Observe(time.Since(m.start).Seconds())
	})
}
