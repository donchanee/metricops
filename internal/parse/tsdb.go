// Package parse reads inputs from disk and returns model types. Each source
// (tsdb, grafana, rules) lives in its own file. Parsers never run analysis;
// they strictly transform text/JSON/YAML into model values.
package parse

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/donchanee/metricops/internal/model"
)

// tsdb analyze section headers the parser recognizes. Unknown sections are
// silently ignored; the parser's job is to be tolerant to promtool version
// drift across 2.48, 2.51, 2.54, and beyond.
const (
	sectHotMetrics = "Highest cardinality metric names:"
	sectHotLabels  = "Highest cardinality labels:"
)

// scannerMaxBuffer caps a single line read from tsdb analyze output.
// Real lines are < 1 KB; 1 MB is a generous ceiling against pathological input.
const scannerMaxBuffer = 1 << 20

// ParseTSDBAnalyze reads the output of `promtool tsdb analyze` and returns
// the list of metrics with their active-series counts.
//
// Parsing strategy: line-oriented state machine. The parser enters the
// "Highest cardinality metric names:" section when it sees that exact
// header, parses each subsequent "<count> <metric>" row, and exits the
// section on any of: blank line, a new section header (line ending ":"),
// or a key-value preamble line ("Foo: bar").
//
// Tolerance:
//   - Unknown sections: skipped.
//   - Malformed data lines: skipped silently (partial results beat none).
//   - Duplicate metric names within the section: first occurrence wins.
//   - Missing "Highest cardinality metric names:" section: returns empty slice.
//
// Errors returned only from I/O (scanner.Err). Never returns an error for
// parse issues in individual lines.
func ParseTSDBAnalyze(r io.Reader) ([]*model.Metric, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), scannerMaxBuffer)

	var metrics []*model.Metric
	seen := make(map[string]struct{})
	section := ""

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		trim := strings.TrimSpace(line)

		// Blank lines end any active section and separate preamble blocks.
		if trim == "" {
			section = ""
			continue
		}

		// Section header: ends with ":" and has no value after.
		if isSectionHeader(trim) {
			section = trim
			continue
		}

		// Key-value header like "Total Series: 76475" also ends a section.
		if isKeyValueHeader(trim) {
			section = ""
			continue
		}

		if section != sectHotMetrics {
			continue
		}

		count, name, ok := parseCountAndName(trim)
		if !ok {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		metrics = append(metrics, &model.Metric{Name: name, ActiveSeries: count})
	}

	if err := scanner.Err(); err != nil {
		return metrics, fmt.Errorf("tsdb analyze scan: %w", err)
	}
	return metrics, nil
}

// ParseTSDBAnalyzeFile opens path and calls ParseTSDBAnalyze. When path is
// "-" it reads from stdin.
func ParseTSDBAnalyzeFile(path string) ([]*model.Metric, error) {
	if path == "-" {
		return ParseTSDBAnalyze(os.Stdin)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tsdb file: %w", err)
	}
	defer f.Close()
	return ParseTSDBAnalyze(f)
}

// isSectionHeader reports whether line ends with a bare colon (section header
// like "Highest cardinality metric names:") vs. a key-value preamble line.
func isSectionHeader(line string) bool {
	return strings.HasSuffix(line, ":") && !strings.Contains(line, ": ")
}

// isKeyValueHeader reports whether line matches "Key: value" form.
// Data rows start with a digit and are explicitly excluded.
func isKeyValueHeader(line string) bool {
	if line == "" {
		return false
	}
	if line[0] >= '0' && line[0] <= '9' {
		return false
	}
	return strings.Contains(line, ": ")
}

// parseCountAndName parses a row like "17234 app_user_actions_total".
// Returns (count, name, true) on success.
func parseCountAndName(line string) (int, string, bool) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(line[:idx])
	if err != nil || n < 0 {
		return 0, "", false
	}
	name := strings.TrimSpace(line[idx+1:])
	if name == "" {
		return 0, "", false
	}
	return n, name, true
}
