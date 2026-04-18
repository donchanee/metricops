# Changelog

All notable changes to metricops are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-04-18

Initial public release. `metricops analyze` produces a governance report
over a self-hosted Prometheus deployment: unused metrics, cardinality
hotspots, and drop-candidate savings.

### Added

- `metricops analyze` subcommand (`metricops validate` and `metricops diff`
  are placeholders reserved for v2).
- Input parsers:
  - `parse.ParseTSDBAnalyze[File]` — `promtool tsdb analyze` output.
  - `parse.ParseGrafanaDir/File` — Grafana v10+ dashboard JSON.
  - `parse.ParseRulesDir/File` — Prometheus alert and recording rule YAML,
    via `github.com/prometheus/prometheus/model/rulefmt`.
- PromQL metric-name extraction via the official Prometheus parser
  (`promqlx.MetricNames`).
- Model builder that joins TSDB metrics with References and tracks
  attribution outcomes (attributed / orphaned / unparseable).
- Detectors:
  - `analyze.DetectUnused` — metrics in TSDB with zero references.
  - `analyze.DetectHotspots` — metrics dominating active series share.
  - `analyze.EstimateSummary` and `EstimateBytesPerDay` — cost math.
  - `analyze.BuildRecommendations` — `drop_metric` actions sorted by
    estimated daily savings.
- Renderers:
  - `render.Markdown` — GitHub-flavored markdown report.
  - `render.JSON` — byte-deterministic v1.0 schema output.
- CLI flags on `analyze`: `--tsdb`, `--grafana`, `--rules`, `--format`,
  `--schema`, `--strict`, `--bytes-per-sample`, `--fail-on`, `--timeout`,
  `--progress`.
- Synthetic fixture generator at `testdata/fixtures/generate.go` with a
  MANIFEST.json ground-truth file.
- GitHub Actions CI (3-OS matrix, lint, fixture determinism).
- GoReleaser-driven release pipeline producing binaries for linux,
  macOS, and windows in amd64 and arm64 (windows arm64 omitted).

### Known limitations

- Recording-rule chains are not traversed (documented in promqlx and
  model docs). A metric referenced only through another recording rule's
  output may appear unused.
- Grafana template-variable expressions (`${var}`, `$__range`) are
  skipped with a stderr warning.
- Grafana repeat panels are skipped.
- Regex `__name__` matchers (`{__name__=~"foo.*"}`) cannot be resolved;
  metrics referenced only this way may appear unused.
- VictoriaMetrics, Thanos, Mimir adapters are not included (v2 target).

[Unreleased]: https://github.com/donchanee/metricops/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/donchanee/metricops/releases/tag/v0.1.0
