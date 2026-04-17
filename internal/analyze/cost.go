package analyze

import (
	"github.com/donchanee/metricops/internal/model"
)

// DefaultBytesPerSample is the default cost constant, in bytes/sample, for
// compressed Prometheus TSDB storage. Based on upstream Prometheus published
// figures; overridable via the --bytes-per-sample flag.
const DefaultBytesPerSample = 2.0

// SampleRate is the assumed scrape rate in samples/second for cost math.
// Default 15s scrape interval = 1/15 samples/s per series. Overridable
// in a future flag if needed; for v1.0 we use the common default.
const SampleRate = 1.0 / 15.0

// secondsPerDay and secondsPerMonth are cost math constants.
const (
	secondsPerDay   = 86400
	secondsPerMonth = secondsPerDay * 30
)

// EstimateBytesPerDay returns the estimated storage cost of a single metric
// per day, in bytes:
//
//	bytes = activeSeries * sampleRate * bytesPerSample * secondsPerDay
//
// Returns 0 (not an error) when activeSeries is 0.
func EstimateBytesPerDay(activeSeries int, bytesPerSample float64) int64 {
	if activeSeries <= 0 {
		return 0
	}
	bps := float64(activeSeries) * SampleRate * bytesPerSample
	return int64(bps * float64(secondsPerDay))
}

// EstimateSummary fills in cost-related fields of a Summary given a model and
// the bytes-per-sample assumption. Never returns an error but the signature
// matches its siblings for consistency.
func EstimateSummary(m *model.Model, bytesPerSample float64) (model.Summary, error) {
	return model.Summary{}, ErrNotImplemented
}
