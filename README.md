# metricops

**Prometheus metric governance for self-hosted operators.**

[![CI](https://github.com/donchanee/metricops/actions/workflows/ci.yml/badge.svg)](https://github.com/donchanee/metricops/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/donchanee/metricops?sort=semver)](https://github.com/donchanee/metricops/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/donchanee/metricops.svg)](https://pkg.go.dev/github.com/donchanee/metricops)
[![License](https://img.shields.io/github/license/donchanee/metricops)](./LICENSE)

`metricops analyze` reads your Prometheus TSDB, Grafana dashboards, and alert
rules and tells you three things you probably can't answer today:

1. **Which metrics are you paying to store but nobody queries?**
2. **Which metrics and labels are quietly eating most of your series budget?**
3. **How many bytes per day would you save if you dropped the dead weight?**

All local. No network calls. No telemetry. No cloud service. Your TSDB
metadata and dashboards never leave the machine `metricops` runs on.

---

## Example output

```markdown
# metricops Report
Generated: 2026-04-17T12:00:00Z · Schema: 1.0

## Summary

| Metric                    | Value                       |
|---------------------------|-----------------------------|
| Total metrics             | 80                          |
| Unused metrics            | 21                          |
| Total active series       | 76,475                      |
| Estimated daily bytes     | 880,991,987 (840.18 MiB)    |
| Estimated monthly bytes   | 26,429,759,610 (24.61 GiB)  |
| Bytes per sample assumed  | 2.00                        |

## Unused Metrics
21 metrics appear in TSDB but are not referenced by any dashboard, alert,
or recording rule.

| Metric                                 | Active Series | Bytes/day    |
|----------------------------------------|--------------:|-------------:|
| `http_request_duration_seconds_bucket` |        15,013 |  172,949,760 |
| `http_response_size_bytes`             |         2,640 |   30,412,800 |
| `legacy_worker_queue_depth`            |            26 |      299,520 |
...

## Recommendations
21 actions identified. Total estimated daily savings: 224.87 MiB.

- `drop_metric` **`http_request_duration_seconds_bucket`** — saves ~164.94 MiB/day
...
```

Ask for JSON with `--format=json` for CI pipelines and scripts. The v1.0 JSON
schema is byte-deterministic across runs (same inputs → byte-identical output,
a CI gate in this repo).

---

## Install

### Go

```bash
go install github.com/donchanee/metricops/cmd/metricops@latest
```

### Pre-built binaries

Grab the archive for your OS and arch from the
[releases page](https://github.com/donchanee/metricops/releases) and put the
`metricops` binary on your `PATH`.

Supported: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.

### Build from source

```bash
git clone https://github.com/donchanee/metricops
cd metricops
go build -o metricops ./cmd/metricops
```

---

## Quick start

You need three inputs:

1. The output of `promtool tsdb analyze <block>` (a text file).
2. Your Grafana dashboards as JSON files in a directory (optional).
3. Your Prometheus rule files as YAML in a directory (optional).

```bash
# Capture TSDB state (takes a minute on a large block; that's promtool's time, not ours)
promtool tsdb analyze /var/lib/prometheus/<block-id> > tsdb-analyze.txt

# Export dashboards (or point at a git-mirrored dashboards-as-code directory)
# Skip this flag if you want a rules-only run.

# Analyze
metricops analyze \
  --tsdb=tsdb-analyze.txt \
  --grafana=./dashboards \
  --rules=/etc/prometheus/rules
```

Try it on synthetic data right now — this repo ships a fixture generator:

```bash
go run testdata/fixtures/generate.go -out ./testdata/fixtures
go run ./cmd/metricops analyze \
  --tsdb=./testdata/fixtures/tsdb-analyze.txt \
  --grafana=./testdata/fixtures/dashboards \
  --rules=./testdata/fixtures/rules
```

---

## CLI reference

```text
metricops analyze [flags]

Flags:
  --tsdb string             promtool tsdb analyze output (file or '-' for stdin) [required]
  --grafana string          Grafana dashboard JSON file or directory
  --rules string            Prometheus rule YAML file or directory
  --format string           output format: markdown (default), md, or json
  --schema string           JSON schema version (default "1.0"; only 1.0 supported in v0.x)
  --strict                  promote per-file parse warnings to exit 1
  --bytes-per-sample float  assumed bytes per sample, compressed TSDB (default 2.00)
  --fail-on string          exit 1 when findings of this kind exist (e.g. 'findings')
  --timeout duration        analysis deadline (default 5m)
  --progress                emit stage-based progress on stderr
```

### Exit codes

| Code | Meaning                                                                |
|------|------------------------------------------------------------------------|
| 0    | Success, with or without findings (unless `--fail-on` matched).        |
| 1    | Findings present and matched `--fail-on`, or strict parse warnings.    |
| 2    | Invalid flags, unreadable input, unsupported schema.                   |

---

## How it works

```text
  ┌───────────────────┐         ┌───────────────────┐
  │  promtool tsdb    │         │  Grafana JSON    +│
  │  analyze output   │         │  Prom rule YAML   │
  └─────────┬─────────┘         └─────────┬─────────┘
            │                             │
            ▼                             ▼
    ┌──────────────┐            ┌───────────────────┐
    │  parse.TSDB  │            │ parse.Grafana/Rules
    └──────┬───────┘            └─────────┬─────────┘
           │ []*Metric                    │ []Reference
           │                              │
           └──────────┬───────────────────┘
                      ▼
             ┌─────────────────┐
             │  builder.Build  │  ← promqlx.MetricNames
             └────────┬────────┘     walks each Expr AST
                      │ *Model        to attribute refs
                      ▼
             ┌─────────────────┐
             │   analyze.*     │  DetectUnused
             │                 │  DetectHotspots
             │                 │  EstimateSummary
             │                 │  BuildRecommendations
             └────────┬────────┘
                      ▼
             ┌─────────────────┐
             │   render.*      │  Markdown or JSON
             └─────────────────┘
                      ▼
                   stdout
```

Every stage is a pure function; parsers do the only I/O. The whole pipeline
runs under `--timeout` (default 5m) so a cron job can't hang your runner.

---

## CI integration

Use `--fail-on=findings` to block PRs that introduce unused metrics or
cardinality hotspots.

```yaml
# .github/workflows/prom-governance.yml
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install metricops
        run: go install github.com/donchanee/metricops/cmd/metricops@latest
      - name: Export TSDB state
        run: |
          ssh prod-prom 'promtool tsdb analyze /var/lib/prometheus/<block>' > tsdb.txt
      - name: Analyze
        run: |
          metricops analyze \
            --tsdb=tsdb.txt \
            --grafana=./dashboards \
            --rules=./rules \
            --format=json \
            --fail-on=findings > report.json
      - uses: actions/upload-artifact@v4
        with:
          name: metricops-report
          path: report.json
```

---

## Data privacy

`metricops` runs entirely on your machine. It does not:

- make outbound network calls,
- phone home with telemetry,
- read configuration from any location you didn't point it at,
- write outputs anywhere other than stdout and stderr.

Your TSDB analysis, dashboards, and rule files stay on disk. You can run
it on air-gapped infrastructure.

---

## Known limitations

- **Recording-rule chains are not traversed.** A metric referenced only
  through another recording rule's output may appear unused. v2 roadmap.
- **Template-variable expressions** (`${var}`, `$__range`) in Grafana are
  skipped with a warning; their metric references are not attributed.
- **Repeat panels** in Grafana are skipped.
- **Regex `__name__` matchers** (`{__name__=~"foo.*"}`) cannot be resolved;
  metrics referenced only this way will appear unused.
- **VictoriaMetrics / Thanos / Mimir** backends are not yet adapted; this
  release targets vanilla Prometheus. v2 adds those via the same parser
  contract.

See [CHANGELOG.md](./CHANGELOG.md) for the full v0.1.0 shipping notes.

---

## Contributing

Issues and PRs welcome.
- Small PRs merge faster. Keep one change per PR.
- `go test ./...` and `go vet ./...` must pass.
- New behavior needs tests.
- The JSON schema is a compatibility surface: additive changes only within
  v1.x; breaking changes bump the major.

---

## License

[Apache-2.0](./LICENSE).
