package daemon

import (
	"sync"

	"control/internal/logging"
)

const itemTimestampMetricsLogEvery = 100

type itemTimestampMetricsSink interface {
	Record(classification itemTimestampClassification)
	Close()
}

type itemTimestampLogMetricsSink struct {
	logger       logging.Logger
	logEvery     uint64
	mu           sync.Mutex
	total        uint64
	providerSeen uint64
	daemonFilled uint64
}

func newItemTimestampLogMetricsSink(logger logging.Logger) *itemTimestampLogMetricsSink {
	if logger == nil {
		logger = logging.Nop()
	}
	return &itemTimestampLogMetricsSink{
		logger:   logger,
		logEvery: itemTimestampMetricsLogEvery,
	}
}

func (s *itemTimestampLogMetricsSink) Record(classification itemTimestampClassification) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.total++
	if classification.HasProviderTimestamp {
		s.providerSeen++
	}
	if classification.UsedDaemonTimestamp {
		s.daemonFilled++
	}
	shouldLog := s.logEvery > 0 && s.total%s.logEvery == 0
	total := s.total
	providerSeen := s.providerSeen
	daemonFilled := s.daemonFilled
	s.mu.Unlock()
	if shouldLog {
		s.emit("periodic", total, providerSeen, daemonFilled)
	}
}

func (s *itemTimestampLogMetricsSink) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	total := s.total
	providerSeen := s.providerSeen
	daemonFilled := s.daemonFilled
	s.mu.Unlock()
	if total == 0 {
		return
	}
	s.emit("final", total, providerSeen, daemonFilled)
}

func (s *itemTimestampLogMetricsSink) emit(mode string, total, providerSeen, daemonFilled uint64) {
	if s == nil || s.logger == nil || total == 0 {
		return
	}
	missingProvider := total - providerSeen
	s.logger.Info(
		"item_timestamp_stats",
		logging.F("mode", mode),
		logging.F("items_total", total),
		logging.F("provider_timestamp_count", providerSeen),
		logging.F("provider_timestamp_pct", percentage(providerSeen, total)),
		logging.F("missing_provider_timestamp_count", missingProvider),
		logging.F("missing_provider_timestamp_pct", percentage(missingProvider, total)),
		logging.F("daemon_filled_count", daemonFilled),
		logging.F("daemon_filled_pct", percentage(daemonFilled, total)),
	)
}

func percentage(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) * 100 / float64(denominator)
}
