package parse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donchanee/metricops/internal/model"
)

// grafanaDashboard is the minimal subset of the Grafana dashboard schema
// this parser cares about. Fields we don't use are omitted; unknown keys
// in the JSON are ignored.
type grafanaDashboard struct {
	UID    string         `json:"uid"`
	Title  string         `json:"title"`
	Panels []grafanaPanel `json:"panels"`
}

// grafanaPanel represents a panel (or a row container with nested panels).
// The datasource and nested panels fields are the key quirks.
type grafanaPanel struct {
	ID         int             `json:"id"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Datasource json.RawMessage `json:"datasource"`
	Targets    []grafanaTarget `json:"targets"`
	// Repeat is set when a panel is dynamically repeated per-variable value.
	// Repeat panels are skipped in MVP per eng review.
	Repeat string `json:"repeat,omitempty"`
	// Panels is populated when this is a row or container panel.
	Panels []grafanaPanel `json:"panels,omitempty"`
}

// grafanaTarget is a single query within a panel. Grafana allows the target
// to override the panel-level datasource, so we capture it too.
type grafanaTarget struct {
	RefID      string          `json:"refId"`
	Expr       string          `json:"expr"`
	Datasource json.RawMessage `json:"datasource"`
}

// ParseGrafanaDir reads all *.json files at the top level of path and
// returns the references found in dashboard panels.
//
// Per eng review, the MVP parser:
//   - supports panel types timeseries, stat, table, bargauge (no filtering,
//     we just extract expr from every panel's targets)
//   - recurses into row panels (which have nested Panels)
//   - skips panels whose Repeat field is set (template repeat)
//   - skips targets whose Expr contains `${` (template variable)
//   - skips targets whose datasource is clearly non-Prometheus
//   - is tolerant to unknown datasource formats (defaults to including)
//
// Per-file parse errors are non-fatal: warn on stderr, continue.
func ParseGrafanaDir(path string) ([]model.Reference, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return ParseGrafanaFile(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}

	var out []model.Reference
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		full := filepath.Join(path, e.Name())
		refs, err := ParseGrafanaFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", full, err)
			continue
		}
		out = append(out, refs...)
	}
	return out, nil
}

// ParseGrafanaFile parses a single dashboard JSON file.
func ParseGrafanaFile(path string) ([]model.Reference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var dash grafanaDashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var refs []model.Reference
	collectPanelRefs(path, nil, dash.Panels, &refs)
	return refs, nil
}

// collectPanelRefs walks panels (recursively into rows) and appends a
// Reference for each Prometheus target.
//
// panelDS is the nearest enclosing datasource; a target inherits it if its
// own datasource field is absent. This handles both panel-level and
// target-level datasource specification.
func collectPanelRefs(path string, panelDS json.RawMessage, panels []grafanaPanel, refs *[]model.Reference) {
	for _, p := range panels {
		// Row or container panels: recurse. Grafana convention is that rows
		// have type "row" with nested Panels, but we rely on presence of
		// nested Panels rather than the type string.
		if len(p.Panels) > 0 {
			effectiveDS := panelDS
			if len(p.Datasource) > 0 {
				effectiveDS = p.Datasource
			}
			collectPanelRefs(path, effectiveDS, p.Panels, refs)
			continue
		}

		// Repeat panels generate series-per-variable-value at render time;
		// their expr often contains template variables. Skipped in MVP.
		if p.Repeat != "" {
			continue
		}

		effectiveDS := panelDS
		if len(p.Datasource) > 0 {
			effectiveDS = p.Datasource
		}

		for _, t := range p.Targets {
			expr := strings.TrimSpace(t.Expr)
			if expr == "" {
				continue
			}
			// Template variable interpolation is out of scope (eng review).
			if strings.Contains(expr, "${") || strings.Contains(expr, "$__") {
				fmt.Fprintf(os.Stderr, "warning: %s#panel:%d target %s: skipping template-variable expr\n",
					path, p.ID, t.RefID)
				continue
			}

			targetDS := t.Datasource
			if len(targetDS) == 0 {
				targetDS = effectiveDS
			}
			if !isPrometheusDatasource(targetDS) {
				continue
			}

			*refs = append(*refs, model.Reference{
				Source:   model.RefDashboard,
				Location: fmt.Sprintf("%s#panel:%d", path, p.ID),
				Expr:     expr,
			})
		}
	}
}

// isPrometheusDatasource reports whether a raw Grafana datasource field
// refers to Prometheus.
//
// Grafana has several formats across versions:
//
//	"datasource": "Prometheus"                          — legacy string
//	"datasource": {"type": "prometheus", "uid": "abc"}  — modern object
//	"datasource": "-- Mixed --"                         — panel uses per-target DS
//	(unset or null)                                     — inherits from dashboard
//
// Policy (eng review): when ambiguous or unknown, default to TRUE so we do
// not silently drop Prometheus references. False negatives here would make
// the tool under-report usage and flag legitimately used metrics as unused.
func isPrometheusDatasource(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	// Null literal: treat as inherited/unknown — default true.
	if string(raw) == "null" {
		return true
	}

	// Object form.
	var obj struct {
		Type string `json:"type"`
		UID  string `json:"uid"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type != "" {
		return strings.EqualFold(obj.Type, "prometheus")
	}

	// String form.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return true
		}
		lower := strings.ToLower(s)
		if strings.Contains(lower, "prometheus") {
			return true
		}
		// Known non-Prometheus datasources: be explicit so we don't emit false refs.
		knownNonProm := []string{"loki", "cloudwatch", "elasticsearch", "influxdb",
			"graphite", "mysql", "postgres", "mssql", "tempo", "jaeger", "zipkin"}
		for _, n := range knownNonProm {
			if strings.Contains(lower, n) {
				return false
			}
		}
		// Unknown string datasource: conservative inclusive default.
		return true
	}

	// Completely unparseable: be inclusive.
	return true
}
