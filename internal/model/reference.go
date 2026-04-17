package model

// RefSource identifies the kind of file that references a metric.
type RefSource string

const (
	RefDashboard RefSource = "dashboard"
	RefAlert     RefSource = "alert"
	RefRecording RefSource = "recording"
)

// Reference is a single usage of a metric in a dashboard panel, alert rule,
// or recording rule. A metric can have many references.
//
// Policy (eng review): a metric is "used" if ANY expression references it,
// even in a recording rule whose output is itself unused. Recording-rule
// chain traversal is NOT performed in MVP; this is by design.
type Reference struct {
	// Source identifies the kind of file.
	Source RefSource

	// Location is a human-readable path plus identifier for rendering. Format:
	//
	//   dashboards/http-01.json#panel:3
	//   rules/rules-01.yml#alert:HighErrorRate
	//   rules/rules-01.yml#record:job:http_requests:rate5m
	//
	// Parsers are responsible for constructing this. The render layer never
	// rebuilds it.
	Location string

	// Expr is the original PromQL expression where the metric appeared.
	// Preserved verbatim so users can see exactly where and how.
	Expr string
}
