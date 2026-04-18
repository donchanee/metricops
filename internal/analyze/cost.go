package analyze

import (
	"github.com/donchanee/metricops/internal/model"
)

// DefaultBytesPerSample is the default cost constant, in bytes/sample, for
// compressed Prometheus TSDB storage. Based on upstream Prometheus published
// figures. Overridable via the --bytes-per-sample flag.
const DefaultBytesPerSample = 2.0

// SampleRate is the assumed scrape rate in samples/second (15s scrape
// interval → 1/15 samples/s per series). Held constant in v1.0; a future
// flag can expose it.
const SampleRate = 1.0 / 15.0

// Time constants for cost math.
const (
	secondsPerDay   = 86400
	secondsPerMonth = secondsPerDay * 30
)

// EstimateBytesPerDay returns the estimated storage cost of a metric per day
// in bytes:
//
//	bytes = activeSeries * SampleRate * bytesPerSample * secondsPerDay
//
// Returns 0 when activeSeries or bytesPerSample is non-positive.
func EstimateBytesPerDay(activeSeries int, bytesPerSample float64) int64 {
	if activeSeries <= 0 || bytesPerSample <= 0 {
		return 0
	}
	bps := float64(activeSeries) * SampleRate * bytesPerSample
	return int64(bps * float64(secondsPerDay))
}

// EstimateSummary fills in the cost- and count-related fields of a Summary
// given a model and the bytes-per-sample assumption.
//
// All summary fields are computed from m; no I/O or external calls. Returns
// a zero Summary (no error) when m is nil.
func EstimateSummary(m *model.Model, bytesPerSample float64) (model.Summary, error) {
	if m == nil {
		return model.Summary{}, nil
	}

	var daily int64
	unused := 0
	for _, metric := range m.Metrics {
		daily += EstimateBytesPerDay(metric.ActiveSeries, bytesPerSample)
		if !metric.IsUsed() {
			unused++
		}
	}

	return model.Summary{
		TotalMetrics:          len(m.Metrics),
		UnusedMetrics:         unused,
		TotalActiveSeries:     m.TotalActiveSeries,
		EstimatedDailyBytes:   daily,
		EstimatedMonthlyBytes: daily * 30,
		BytesPerSampleAssumed: bytesPerSample,
	}, nil
}
