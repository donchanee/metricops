# metricops

Prometheus metric governance for self-hosted operators.

`metricops` analyzes a self-hosted Prometheus deployment and reports:

- **Unused metrics** — collected by TSDB but referenced by no dashboard, alert, or recording rule
- **Cardinality hotspots** — metrics and labels dominating active series count
- **Cost estimates** — bytes per day per metric, with drop-candidate savings

It runs as a local CLI, consumes files on disk, and writes a report to stdout.
No network calls. No telemetry. No cloud dependency.

## Status

Pre-release. Under active development. Not yet suitable for production use.
Follow [github.com/\<owner\>/metricops/releases](.) for v0.1.0.

## Install

Requires Go 1.22+.

```bash
go install metricops/cmd/metricops@latest
```

Pre-built binaries will be published via GitHub Releases from v0.1.0.

## Quick start

```bash
# Produce a fixture (once)
go run testdata/fixtures/generate.go -out ./testdata/fixtures

# Analyze the fixture
metricops analyze \
  --tsdb=./testdata/fixtures/tsdb-analyze.txt \
  --grafana=./testdata/fixtures/dashboards \
  --rules=./testdata/fixtures/rules
```

## Against your own Prometheus

```bash
# 1. capture tsdb analyze output
promtool tsdb analyze /path/to/prom/data > tsdb-analyze.txt

# 2. export dashboards (or point at a git-mirrored directory)
#    you can skip this flag if you want a rules-only analysis

# 3. run
metricops analyze --tsdb=tsdb-analyze.txt \
                  --grafana=/path/to/dashboards \
                  --rules=/path/to/prometheus/rules
```

## Data privacy

`metricops` runs entirely on your machine. No data is uploaded anywhere.
Your TSDB analysis, dashboards, and rules stay on disk.

## License

Apache-2.0.
