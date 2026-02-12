package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type bucket struct {
	Statements int64   `json:"statements"`
	Covered    int64   `json:"covered"`
	Percent    float64 `json:"percent"`
}

type summary struct {
	GeneratedAt       string                 `json:"generated_at"`
	CoverageAvailable bool                   `json:"coverage_available"`
	CoverageHTML      string                 `json:"coverage_html"`
	Overall           bucket                 `json:"overall"`
	Threshold         float64                `json:"threshold"`
	Pass              bool                   `json:"pass"`
	Files             map[string]bucket      `json:"files"`
	Folders           map[string]bucket      `json:"folders"`
	Tests             testSummary            `json:"tests"`
	Links             map[string]string      `json:"links,omitempty"`
	Meta              map[string]interface{} `json:"meta,omitempty"`
}

type coverageBlock struct {
	Path       string
	Statements int64
	Covered    bool
}

type kv struct {
	Name   string
	Bucket bucket
}

type goTestEvent struct {
	Action      string  `json:"Action"`
	Package     string  `json:"Package"`
	Test        string  `json:"Test"`
	Elapsed     float64 `json:"Elapsed"`
	Output      string  `json:"Output"`
	FailedBuild string  `json:"FailedBuild"`
}

type failedTest struct {
	Package string   `json:"package"`
	Test    string   `json:"test"`
	Elapsed float64  `json:"elapsed"`
	Output  []string `json:"output,omitempty"`
}

type failedPackage struct {
	Package string   `json:"package"`
	Elapsed float64  `json:"elapsed"`
	Output  []string `json:"output,omitempty"`
}

type packageSummary struct {
	Status  string  `json:"status"`
	Elapsed float64 `json:"elapsed"`
}

type testSummary struct {
	Total          int                       `json:"total"`
	Passed         int                       `json:"passed"`
	Failed         int                       `json:"failed"`
	Skipped        int                       `json:"skipped"`
	Pass           bool                      `json:"pass"`
	Packages       map[string]packageSummary `json:"packages"`
	FailedTests    []failedTest              `json:"failed_tests"`
	FailedPackages []failedPackage           `json:"failed_packages"`
}

type dashboardData struct {
	Summary     *summary
	Folders     []kv
	Files       []kv
	LowFiles    []kv
	FailedTests []failedTest
	FailedPkgs  []failedPackage
	GeneratedAt string
}

func main() {
	in := flag.String("in", "coverage/coverage.out", "input coverage profile")
	outJSON := flag.String("out-json", "coverage/summary.json", "output summary json")
	outMD := flag.String("out-md", "coverage/summary.md", "output summary markdown")
	outHTML := flag.String("out-html", "coverage/index.html", "output dashboard html")
	coverageHTML := flag.String("coverage-html", "coverage/coverage.html", "go tool cover html path")
	testJSON := flag.String("test-json", "coverage/test-report.jsonl", "go test -json output file")
	excludeFiles := flag.String("exclude-files", "", "comma-separated file patterns to exclude from coverage summary (supports glob)")
	threshold := flag.Float64("threshold", 70.0, "minimum overall coverage percent")
	enforceTests := flag.Bool("enforce-tests", false, "exit non-zero when tests fail")
	enforce := flag.Bool("enforce", false, "exit non-zero when below threshold")
	flag.Parse()

	s := &summary{
		GeneratedAt:       time.Now().Format(time.RFC3339),
		CoverageAvailable: false,
		CoverageHTML:      filepath.Base(*coverageHTML),
		Threshold:         *threshold,
		Pass:              false,
		Files:             map[string]bucket{},
		Folders:           map[string]bucket{},
		Links:             map[string]string{},
	}

	coverageSummary, err := buildSummary(*in, *threshold, parseExcludePatterns(*excludeFiles))
	if err == nil {
		s = coverageSummary
		s.CoverageHTML = filepath.Base(*coverageHTML)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "coverage summary error: %v\n", err)
		os.Exit(1)
	}

	tests, err := parseTestJSON(*testJSON)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "test summary error: %v\n", err)
		os.Exit(1)
	}
	s.Tests = tests
	s.Pass = s.CoverageAvailable && s.Overall.Percent >= s.Threshold
	if !s.CoverageAvailable {
		s.Pass = false
	}
	s.Links = parseCoverageHTMLLinks(*coverageHTML, moduleName(), s.CoverageHTML)

	if err := os.MkdirAll(filepath.Dir(*outJSON), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir output dir error: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(*outJSON, s); err != nil {
		fmt.Fprintf(os.Stderr, "write json error: %v\n", err)
		os.Exit(1)
	}
	if err := writeMarkdown(*outMD, s); err != nil {
		fmt.Fprintf(os.Stderr, "write markdown error: %v\n", err)
		os.Exit(1)
	}
	if err := writeDashboard(*outHTML, s); err != nil {
		fmt.Fprintf(os.Stderr, "write html dashboard error: %v\n", err)
		os.Exit(1)
	}

	printSummary(s)
	if *enforce && (!s.Pass || !s.Tests.Pass) {
		os.Exit(2)
	}
	if *enforceTests && !s.Tests.Pass {
		os.Exit(3)
	}
}

func buildSummary(profilePath string, threshold float64, excludePatterns []string) (*summary, error) {
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	module := moduleName()
	files := map[string]bucket{}
	folders := map[string]bucket{}
	blocks := map[string]coverageBlock{}

	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(line, "mode:") {
				continue
			}
		}

		path, rangeKey, stmts, covered, err := parseProfileLine(line)
		if err != nil {
			return nil, err
		}

		rel := normalizePath(path, module)
		if isExcluded(rel, excludePatterns) {
			continue
		}
		// go test ./... with -coverpkg can emit duplicated blocks for the same source file
		// from multiple test binaries, sometimes with shifted line numbers.
		// Normalize block keys by structural shape to avoid under-reporting.
		shapeKey := normalizedBlockShape(rangeKey, stmts)
		key := rel + ":" + shapeKey
		existing, ok := blocks[key]
		if !ok {
			blocks[key] = coverageBlock{
				Path:       rel,
				Statements: stmts,
				Covered:    covered > 0,
			}
			continue
		}
		if covered > 0 {
			existing.Covered = true
			blocks[key] = existing
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	var totalStmts int64
	var totalCovered int64
	for _, b := range blocks {
		dir := filepath.ToSlash(filepath.Dir(b.Path))

		fb := files[b.Path]
		fb.Statements += b.Statements
		if b.Covered {
			fb.Covered += b.Statements
		}
		files[b.Path] = fb

		db := folders[dir]
		db.Statements += b.Statements
		if b.Covered {
			db.Covered += b.Statements
		}
		folders[dir] = db

		totalStmts += b.Statements
		if b.Covered {
			totalCovered += b.Statements
		}
	}

	for k, v := range files {
		v.Percent = pct(v.Covered, v.Statements)
		files[k] = v
	}
	for k, v := range folders {
		v.Percent = pct(v.Covered, v.Statements)
		folders[k] = v
	}

	overall := bucket{
		Statements: totalStmts,
		Covered:    totalCovered,
		Percent:    pct(totalCovered, totalStmts),
	}
	return &summary{
		GeneratedAt:       time.Now().Format(time.RFC3339),
		CoverageAvailable: totalStmts > 0,
		CoverageHTML:      "coverage.html",
		Overall:           overall,
		Threshold:         threshold,
		Pass:              overall.Percent >= threshold,
		Files:             files,
		Folders:           folders,
		Links:             map[string]string{},
	}, nil
}

func normalizedBlockShape(rangeKey string, stmts int64) string {
	start, end, ok := parseRangeCoords(rangeKey)
	if !ok {
		return rangeKey + ":" + strconv.FormatInt(stmts, 10)
	}
	lineSpan := end.line - start.line
	colSpan := end.col - start.col
	return fmt.Sprintf("ls:%d,sc:%d,ec:%d,cs:%d,s:%d", lineSpan, start.col, end.col, colSpan, stmts)
}

type rangeCoord struct {
	line int
	col  int
}

func parseRangeCoords(rangeKey string) (rangeCoord, rangeCoord, bool) {
	parts := strings.Split(rangeKey, ",")
	if len(parts) != 2 {
		return rangeCoord{}, rangeCoord{}, false
	}
	start, ok := parseCoord(parts[0])
	if !ok {
		return rangeCoord{}, rangeCoord{}, false
	}
	end, ok := parseCoord(parts[1])
	if !ok {
		return rangeCoord{}, rangeCoord{}, false
	}
	return start, end, true
}

func parseCoord(part string) (rangeCoord, bool) {
	p := strings.TrimSpace(part)
	dot := strings.Index(p, ".")
	if dot == -1 {
		return rangeCoord{}, false
	}
	line, err := strconv.Atoi(strings.TrimSpace(p[:dot]))
	if err != nil {
		return rangeCoord{}, false
	}
	col, err := strconv.Atoi(strings.TrimSpace(p[dot+1:]))
	if err != nil {
		return rangeCoord{}, false
	}
	return rangeCoord{line: line, col: col}, true
}

func parseExcludePatterns(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	patterns := make([]string, 0, len(parts))
	for _, part := range parts {
		p := filepath.ToSlash(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns
}

func isExcluded(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	norm := filepath.ToSlash(strings.TrimSpace(path))
	for _, pattern := range patterns {
		pat := filepath.ToSlash(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		// Directory-style excludes: "internal/modules/auth/"
		if strings.HasSuffix(pat, "/") && strings.HasPrefix(norm, pat) {
			return true
		}
		// Prefix excludes without glob: "internal/modules/auth"
		if !strings.ContainsAny(pat, "*?[") {
			if norm == pat || strings.HasPrefix(norm, pat+"/") {
				return true
			}
		}
		if pat == norm {
			return true
		}
		matched, err := filepath.Match(pat, norm)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func parseProfileLine(line string) (path string, rangeKey string, stmts int64, covered int64, err error) {
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return "", "", 0, 0, fmt.Errorf("invalid profile line: %s", line)
	}

	pathAndRange := parts[0]
	lastColon := strings.LastIndex(pathAndRange, ":")
	if lastColon == -1 {
		return "", "", 0, 0, fmt.Errorf("invalid path/range: %s", line)
	}
	path = pathAndRange[:lastColon]
	rangeKey = pathAndRange[lastColon+1:]

	if _, err := fmt.Sscanf(parts[1], "%d", &stmts); err != nil {
		return "", "", 0, 0, fmt.Errorf("invalid statements in line: %s", line)
	}
	var count int64
	if _, err := fmt.Sscanf(parts[2], "%d", &count); err != nil {
		return "", "", 0, 0, fmt.Errorf("invalid count in line: %s", line)
	}
	if count > 0 {
		covered = stmts
	}
	return path, rangeKey, stmts, covered, nil
}

func normalizePath(path, module string) string {
	p := filepath.ToSlash(path)
	if module != "" {
		prefix := module + "/"
		if strings.HasPrefix(p, prefix) {
			return strings.TrimPrefix(p, prefix)
		}
	}
	if idx := strings.Index(p, "/internal/"); idx != -1 {
		return strings.TrimPrefix(p[idx+1:], "/")
	}
	if idx := strings.Index(p, "/cmd/"); idx != -1 {
		return strings.TrimPrefix(p[idx+1:], "/")
	}
	if idx := strings.Index(p, "/pkg/"); idx != -1 {
		return strings.TrimPrefix(p[idx+1:], "/")
	}
	return p
}

func moduleName() string {
	f, err := os.Open("go.mod")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func pct(covered, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) * 100 / float64(total)
}

func writeJSON(path string, s *summary) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeMarkdown(path string, s *summary) error {
	var b strings.Builder
	b.WriteString("# Quality Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated: **%s**\n", s.GeneratedAt))
	if s.CoverageAvailable {
		b.WriteString(fmt.Sprintf("- Coverage Overall: **%.2f%%** (%d/%d statements)\n", s.Overall.Percent, s.Overall.Covered, s.Overall.Statements))
	} else {
		b.WriteString("- Coverage Overall: **N/A** (coverage profile missing)\n")
	}
	b.WriteString(fmt.Sprintf("- Threshold: **%.2f%%**\n", s.Threshold))
	b.WriteString(fmt.Sprintf("- Coverage Status: **%s**\n", map[bool]string{true: "PASS", false: "FAIL"}[s.Pass]))
	b.WriteString(fmt.Sprintf("- Test Status: **%s** (%d total, %d passed, %d failed, %d skipped)\n\n",
		map[bool]string{true: "PASS", false: "FAIL"}[s.Tests.Pass], s.Tests.Total, s.Tests.Passed, s.Tests.Failed, s.Tests.Skipped))

	if len(s.Tests.FailedTests) > 0 || len(s.Tests.FailedPackages) > 0 {
		b.WriteString("## Failures\n\n")
		for _, ft := range s.Tests.FailedTests {
			b.WriteString(fmt.Sprintf("- Test: `%s/%s` (%.2fs)\n", ft.Package, ft.Test, ft.Elapsed))
		}
		for _, fp := range s.Tests.FailedPackages {
			b.WriteString(fmt.Sprintf("- Package: `%s` (%.2fs)\n", fp.Package, fp.Elapsed))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Folder Coverage\n\n")
	b.WriteString("| Folder | Coverage |\n|---|---:|\n")
	for _, item := range sortedKVs(s.Folders) {
		b.WriteString(fmt.Sprintf("| `%s` | %s %.2f%% |\n", item.Name, coverageLabel(item.Bucket.Percent), item.Bucket.Percent))
	}

	b.WriteString("\n## Lowest-Coverage Files (Top 20)\n\n")
	b.WriteString("| File | Coverage |\n|---|---:|\n")
	files := sortedKVs(s.Files)
	limit := 20
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		item := files[i]
		b.WriteString(fmt.Sprintf("| `%s` | %s %.2f%% |\n", item.Name, coverageLabel(item.Bucket.Percent), item.Bucket.Percent))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func printSummary(s *summary) {
	if s.CoverageAvailable {
		fmt.Printf("Overall coverage: %s%.2f%%%s (%d/%d)\n", coverageANSI(s.Overall.Percent), s.Overall.Percent, ansiReset, s.Overall.Covered, s.Overall.Statements)
	} else {
		fmt.Printf("Overall coverage: %sN/A%s (coverage profile missing)\n", ansiRed, ansiReset)
	}
	fmt.Printf("Threshold: %.2f%% -> %s%s%s\n", s.Threshold, statusANSI(s.Pass), map[bool]string{true: "PASS", false: "FAIL"}[s.Pass], ansiReset)
	fmt.Printf("Tests: %s%s%s (%d total, %d passed, %d failed, %d skipped)\n",
		statusANSI(s.Tests.Pass), map[bool]string{true: "PASS", false: "FAIL"}[s.Tests.Pass], ansiReset,
		s.Tests.Total, s.Tests.Passed, s.Tests.Failed, s.Tests.Skipped)

	fmt.Println("\nFolder coverage:")
	for _, item := range sortedKVs(s.Folders) {
		fmt.Printf("  %-30s %s%6.2f%%%s\n", item.Name, coverageANSI(item.Bucket.Percent), item.Bucket.Percent, ansiReset)
	}

	fmt.Println("\nLowest-coverage files:")
	files := sortedKVs(s.Files)
	limit := 10
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		item := files[i]
		fmt.Printf("  %-50s %s%6.2f%%%s\n", item.Name, coverageANSI(item.Bucket.Percent), item.Bucket.Percent, ansiReset)
	}

	if len(s.Tests.FailedTests) > 0 || len(s.Tests.FailedPackages) > 0 {
		fmt.Println("\nFailures:")
		for _, ft := range s.Tests.FailedTests {
			fmt.Printf("  %sFAIL%s %s/%s (%.2fs)\n", ansiRed, ansiReset, ft.Package, ft.Test, ft.Elapsed)
			for _, line := range ft.Output {
				fmt.Printf("    %s\n", line)
			}
		}
		for _, fp := range s.Tests.FailedPackages {
			fmt.Printf("  %sFAIL%s %s (package, %.2fs)\n", ansiRed, ansiReset, fp.Package, fp.Elapsed)
			for _, line := range fp.Output {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	fmt.Println("\nArtifacts:")
	fmt.Println("  - coverage/summary.md")
	fmt.Println("  - coverage/summary.json")
	fmt.Println("  - coverage/coverage.html")
	fmt.Println("  - coverage/coverage-details.html")
}

func sortedKVs(m map[string]bucket) []kv {
	items := make([]kv, 0, len(m))
	for k, v := range m {
		items = append(items, kv{Name: k, Bucket: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Bucket.Percent == items[j].Bucket.Percent {
			return items[i].Name < items[j].Name
		}
		return items[i].Bucket.Percent < items[j].Bucket.Percent
	})
	return items
}

func parseTestJSON(path string) (testSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return testSummary{Pass: true, Packages: map[string]packageSummary{}}, err
	}
	defer f.Close()

	tests := testSummary{
		Pass:     true,
		Packages: map[string]packageSummary{},
	}
	testOutput := map[string][]string{}
	pkgOutput := map[string][]string{}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Package == "" {
			continue
		}

		ps := tests.Packages[ev.Package]
		switch ev.Action {
		case "pass", "fail", "skip":
			if ev.Test == "" {
				ps.Status = ev.Action
				ps.Elapsed = ev.Elapsed
				tests.Packages[ev.Package] = ps
				if ev.Action == "fail" {
					tests.Pass = false
				}
				continue
			}

			tests.Total++
			switch ev.Action {
			case "pass":
				tests.Passed++
			case "skip":
				tests.Skipped++
			case "fail":
				tests.Failed++
				tests.Pass = false
				key := ev.Package + "/" + ev.Test
				tests.FailedTests = append(tests.FailedTests, failedTest{
					Package: ev.Package,
					Test:    ev.Test,
					Elapsed: ev.Elapsed,
					Output:  trimOutput(testOutput[key], 8),
				})
			}
		case "output":
			out := strings.TrimSpace(ev.Output)
			if out == "" {
				continue
			}
			if ev.Test != "" {
				key := ev.Package + "/" + ev.Test
				testOutput[key] = append(testOutput[key], out)
			} else {
				pkgOutput[ev.Package] = append(pkgOutput[ev.Package], out)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return tests, err
	}

	for pkg, ps := range tests.Packages {
		if ps.Status == "fail" {
			tests.FailedPackages = append(tests.FailedPackages, failedPackage{
				Package: pkg,
				Elapsed: ps.Elapsed,
				Output:  trimOutput(pkgOutput[pkg], 10),
			})
		}
	}
	sort.Slice(tests.FailedTests, func(i, j int) bool {
		if tests.FailedTests[i].Package == tests.FailedTests[j].Package {
			return tests.FailedTests[i].Test < tests.FailedTests[j].Test
		}
		return tests.FailedTests[i].Package < tests.FailedTests[j].Package
	})
	sort.Slice(tests.FailedPackages, func(i, j int) bool {
		return tests.FailedPackages[i].Package < tests.FailedPackages[j].Package
	})
	return tests, nil
}

func trimOutput(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	start := len(lines) - max
	out := make([]string, 0, max+1)
	out = append(out, fmt.Sprintf("... (%d lines omitted) ...", start))
	out = append(out, lines[start:]...)
	return out
}

func parseCoverageHTMLLinks(path, module, htmlName string) map[string]string {
	links := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return links
	}
	re := regexp.MustCompile(`<option value="(file\d+)">([^<]+) \([0-9.]+%\)</option>`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	for _, m := range matches {
		if len(m) != 3 {
			continue
		}
		rel := normalizePath(strings.TrimSpace(m[2]), module)
		links[rel] = htmlName + "#" + m[1]
	}
	return links
}

func writeDashboard(path string, s *summary) error {
	files := sortedKVs(s.Files)
	low := files
	if len(low) > 20 {
		low = low[:20]
	}
	data := dashboardData{
		Summary:     s,
		Folders:     sortedKVs(s.Folders),
		Files:       files,
		LowFiles:    low,
		FailedTests: s.Tests.FailedTests,
		FailedPkgs:  s.Tests.FailedPackages,
		GeneratedAt: s.GeneratedAt,
	}

	const tpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Coverage Dashboard</title>
  <style>
    :root { --bg:#0b1020; --card:#111a33; --text:#e8eefc; --muted:#94a3b8; --good:#22c55e; --warn:#f59e0b; --bad:#ef4444; --line:#233055; }
    body { margin:0; font-family: "Segoe UI", Tahoma, sans-serif; background:linear-gradient(180deg,#070c1b,#0b1020); color:var(--text); }
    .wrap { max-width:1200px; margin:0 auto; padding:20px; }
    .row { display:grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap:12px; margin-bottom:16px; }
    .card { background:var(--card); border:1px solid var(--line); border-radius:10px; padding:14px; }
    .label { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.08em; }
    .value { font-size:24px; font-weight:700; margin-top:6px; }
    .ok { color:var(--good); } .warn { color:var(--warn); } .bad { color:var(--bad); }
    a { color:#93c5fd; text-decoration:none; } a:hover { text-decoration:underline; }
    h2 { margin:22px 0 10px; font-size:18px; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th,td { text-align:left; padding:8px; border-bottom:1px solid var(--line); vertical-align:top; }
    th:last-child, td:last-child { text-align:right; }
    .pill { display:inline-block; min-width:64px; text-align:center; border-radius:999px; padding:2px 8px; font-weight:600; }
    .bar { height:8px; border-radius:999px; background:#0a132b; overflow:hidden; border:1px solid var(--line); }
    .fill { height:100%; }
    .meta { color:var(--muted); font-size:12px; margin:8px 0 16px; }
    .list { margin:0; padding-left:18px; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>Coverage Dashboard</h1>
    <div class="meta">Generated at {{.GeneratedAt}} | <a href="{{.Summary.CoverageHTML}}">Open line-by-line coverage</a> | <a href="summary.md">Markdown report</a></div>
    <div class="row">
      <div class="card">
        <div class="label">Coverage Overall</div>
        <div class="value {{statusClass .Summary.Overall.Percent}}">
          {{if .Summary.CoverageAvailable}}{{printf "%.2f%%" .Summary.Overall.Percent}}{{else}}N/A{{end}}
        </div>
      </div>
      <div class="card">
        <div class="label">Coverage Threshold</div>
        <div class="value">{{printf "%.2f%%" .Summary.Threshold}}</div>
      </div>
      <div class="card">
        <div class="label">Coverage Status</div>
        <div class="value {{if .Summary.Pass}}ok{{else}}bad{{end}}">{{if .Summary.Pass}}PASS{{else}}FAIL{{end}}</div>
      </div>
      <div class="card">
        <div class="label">Tests</div>
        <div class="value {{if .Summary.Tests.Pass}}ok{{else}}bad{{end}}">{{if .Summary.Tests.Pass}}PASS{{else}}FAIL{{end}}</div>
      </div>
    </div>

    <h2>Folder Coverage</h2>
    <table>
      <thead><tr><th>Folder</th><th>Progress</th><th>Coverage</th></tr></thead>
      <tbody>
      {{range .Folders}}
      <tr>
        <td><code>{{.Name}}</code></td>
        <td>
          <div class="bar"><div class="fill {{statusClass .Bucket.Percent}}" style="width: {{printf "%.2f" .Bucket.Percent}}%; background: {{statusColor .Bucket.Percent}};"></div></div>
        </td>
        <td><span class="pill {{statusClass .Bucket.Percent}}">{{printf "%.2f%%" .Bucket.Percent}}</span></td>
      </tr>
      {{end}}
      </tbody>
    </table>

    <h2>Lowest-Coverage Files</h2>
    <table>
      <thead><tr><th>File</th><th>Progress</th><th>Coverage</th></tr></thead>
      <tbody>
      {{range .LowFiles}}
      <tr>
        <td><a href="{{fileLink $.Summary .Name}}"><code>{{.Name}}</code></a></td>
        <td>
          <div class="bar"><div class="fill {{statusClass .Bucket.Percent}}" style="width: {{printf "%.2f" .Bucket.Percent}}%; background: {{statusColor .Bucket.Percent}};"></div></div>
        </td>
        <td><span class="pill {{statusClass .Bucket.Percent}}">{{printf "%.2f%%" .Bucket.Percent}}</span></td>
      </tr>
      {{end}}
      </tbody>
    </table>

    <h2>Failing Tests</h2>
    {{if and (eq (len .FailedTests) 0) (eq (len .FailedPkgs) 0)}}
    <div class="card ok">No failing tests.</div>
    {{else}}
    <table>
      <thead><tr><th>Target</th><th>Type</th><th>Details</th></tr></thead>
      <tbody>
      {{range .FailedTests}}
      <tr>
        <td><code>{{.Package}}/{{.Test}}</code></td>
        <td><span class="pill bad">test</span></td>
        <td>
          {{if .Output}}
          <ul class="list">
            {{range .Output}}<li><code>{{.}}</code></li>{{end}}
          </ul>
          {{else}}-{{end}}
        </td>
      </tr>
      {{end}}
      {{range .FailedPkgs}}
      <tr>
        <td><code>{{.Package}}</code></td>
        <td><span class="pill bad">package</span></td>
        <td>
          {{if .Output}}
          <ul class="list">
            {{range .Output}}<li><code>{{.}}</code></li>{{end}}
          </ul>
          {{else}}-{{end}}
        </td>
      </tr>
      {{end}}
      </tbody>
    </table>
    {{end}}
  </div>
</body>
</html>`

	funcMap := template.FuncMap{
		"statusClass": func(p float64) string {
			switch {
			case p >= 80:
				return "ok"
			case p >= 60:
				return "warn"
			default:
				return "bad"
			}
		},
		"statusColor": func(p float64) string {
			switch {
			case p >= 80:
				return "#22c55e"
			case p >= 60:
				return "#f59e0b"
			default:
				return "#ef4444"
			}
		},
		"fileLink": func(s *summary, file string) string {
			if link, ok := s.Links[file]; ok {
				return link
			}
			return s.CoverageHTML
		},
	}

	t, err := template.New("dashboard").Funcs(funcMap).Parse(tpl)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}

func coverageLabel(p float64) string {
	switch {
	case p >= 80:
		return "[GREEN]"
	case p >= 60:
		return "[YELLOW]"
	default:
		return "[RED]"
	}
}

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

func coverageANSI(p float64) string {
	switch {
	case p >= 80:
		return ansiGreen
	case p >= 60:
		return ansiYellow
	default:
		return ansiRed
	}
}

func statusANSI(pass bool) string {
	if pass {
		return ansiGreen
	}
	return ansiRed
}
