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

func (m *Metrics) Stop() {
	m.once.Do(func() {
		ConsensusDuration.WithLabelValues(m.src, m.ig).Observe(time.Since(m.start).Seconds())
	})
}
