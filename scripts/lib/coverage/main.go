// Package main provides a Go-based coverage analysis tool.
// It runs tests, collects coverage data, and generates comprehensive reports.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Config holds the configuration for coverage analysis
type Config struct {
	// Thresholds
	ThresholdFunc    float64
	ThresholdPackage float64
	ThresholdPrint   float64
	ThresholdTotal   float64

	// Behavior
	CIMode          bool   // Enable CI mode (GitHub Actions error format, fail on CRITICAL)
	RaceDetection   bool   // Enable -race flag
	ExcludePackages string // Packages to exclude (comma-separated)
	CoverProfile    string // Coverage profile path
	UncoveredLimit  int    // Max uncovered blocks to show
	ShowTestCounts  bool   // Show TESTS column in package summary
}

const ModulePrefix = "github.com/syntrixbase/syntrix/"

// ANSI color codes
const (
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorReset  = "\033[0m"
)

// TestEvent represents a single JSON event from go test -json
type TestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// PackageResult holds test results for a package
type PackageResult struct {
	Name        string
	Status      string // "ok", "FAIL", "?"
	Duration    string
	Coverage    float64
	CoverageStr string
	TestCount   int
	Cached      bool
}

// FuncCoverage represents function-level coverage
type FuncCoverage struct {
	Location string
	Function string
	Coverage float64
}

// Global config
var cfg Config

func main() {
	cfg = parseConfig()

	fmt.Println()

	// Run tests and collect results
	results, testCounts, exitCode := runTests()
	if exitCode != 0 && len(results) == 0 {
		os.Exit(exitCode)
	}

	// Print package coverage summary
	hasCriticalPackage := printPackageSummary(results, testCounts)

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	// Get function coverage data
	funcData := getFunctionCoverage()

	// Print function coverage details
	hasCriticalFunc := printFunctionCoverage(funcData)

	// Print total and check threshold
	hasCriticalTotal := printTotal()

	// Print statistics
	printStatistics(funcData, testCounts)

	// Generate HTML report (only in local mode)
	if !cfg.CIMode {
		generateHTMLReport()
	}

	// Analyze uncovered blocks
	fmt.Println()
	fmt.Println(strings.Repeat("-", 90))
	hasCriticalBlocks := analyzeUncoveredBlocks()

	// In CI mode, exit with error if any CRITICAL issues
	if cfg.CIMode && (hasCriticalPackage || hasCriticalFunc || hasCriticalTotal || hasCriticalBlocks) {
		os.Exit(1)
	}
}

// parseConfig parses configuration from environment variables and arguments
func parseConfig() Config {
	c := Config{
		ThresholdFunc:    80.0,
		ThresholdPackage: 85.0,
		ThresholdPrint:   85.0,
		ThresholdTotal:   90.0,
		CIMode:           os.Getenv("CI") == "true",
		RaceDetection:    false,
		ExcludePackages:  "syntrix/cmd/,syntrix/api/,syntrix/scripts/",
		CoverProfile:     "/tmp/coverage.out",
		UncoveredLimit:   10,
		ShowTestCounts:   true,
	}

	// CI mode has different defaults
	if c.CIMode {
		c.ThresholdPrint = 90.0
		c.RaceDetection = true
		c.CoverProfile = "coverage.out"
		c.UncoveredLimit = 20
		c.ShowTestCounts = false
	}

	// Override from environment
	if v := os.Getenv("COVERPROFILE"); v != "" {
		c.CoverProfile = v
	}
	if v := os.Getenv("THRESHOLD_FUNC"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.ThresholdFunc = f
		}
	}
	if v := os.Getenv("THRESHOLD_PACKAGE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.ThresholdPackage = f
		}
	}
	if v := os.Getenv("THRESHOLD_PRINT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.ThresholdPrint = f
		}
	}
	if v := os.Getenv("THRESHOLD_TOTAL"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.ThresholdTotal = f
		}
	}
	if v := os.Getenv("UNCOVERED_LIMIT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			c.UncoveredLimit = i
		}
	}

	return c
}

// runTests executes go test and parses JSON output
func runTests() ([]PackageResult, map[string]int, int) {
	// Build package list
	pkgCmd := exec.Command("go", "list", "./...")
	pkgOutput, err := pkgCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing packages: %v\n", err)
		return nil, nil, 1
	}

	var pkgs []string
	excludes := strings.Split(cfg.ExcludePackages, ",")
	for _, pkg := range strings.Split(strings.TrimSpace(string(pkgOutput)), "\n") {
		excluded := false
		for _, ex := range excludes {
			if ex != "" && strings.Contains(pkg, ex) {
				excluded = true
				break
			}
		}
		if !excluded {
			pkgs = append(pkgs, pkg)
		}
	}

	args := []string{"test"}
	args = append(args, pkgs...)
	args = append(args, "-json", "-covermode=atomic", "-coverprofile="+cfg.CoverProfile)
	if cfg.RaceDetection {
		args = append(args, "-race")
	}

	cmd := exec.Command("go", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stdout pipe: %v\n", err)
		return nil, nil, 1
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting tests: %v\n", err)
		return nil, nil, 1
	}

	results, testCounts := parseTestOutput(stdout)

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return results, testCounts, exitCode
}

// parseTestOutput parses JSON output from go test
func parseTestOutput(r io.Reader) ([]PackageResult, map[string]int) {
	results := make(map[string]*PackageResult)
	testCounts := make(map[string]int)
	decoder := json.NewDecoder(r)

	for {
		var event TestEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		pkg := strings.TrimPrefix(event.Package, ModulePrefix)

		// Count top-level tests (tests without "/" in name)
		if event.Action == "run" && event.Test != "" && !strings.Contains(event.Test, "/") {
			testCounts[pkg]++
		}

		// Parse coverage from output lines
		if event.Action == "output" && strings.HasPrefix(event.Output, "ok") {
			parsePackageResult(event.Output, results)
		} else if event.Action == "output" && strings.HasPrefix(event.Output, "FAIL") {
			parsePackageResult(event.Output, results)
		} else if event.Action == "output" && strings.HasPrefix(event.Output, "?") {
			parsePackageResult(event.Output, results)
		}
	}

	// Convert map to slice
	var resultSlice []PackageResult
	for _, r := range results {
		resultSlice = append(resultSlice, *r)
	}

	return resultSlice, testCounts
}

// parsePackageResult parses a single package result line
func parsePackageResult(line string, results map[string]*PackageResult) {
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}

	status := fields[0]
	pkg := strings.TrimPrefix(fields[1], ModulePrefix)

	// Skip test packages in display
	if strings.HasPrefix(pkg, "tests/") {
		return
	}

	result := &PackageResult{
		Name:   pkg,
		Status: status,
	}

	if status == "ok" && len(fields) >= 3 {
		result.Duration = fields[2]
		result.Cached = strings.Contains(line, "(cached)")

		// Parse coverage percentage
		for _, f := range fields {
			if strings.HasSuffix(f, "%") {
				f = strings.TrimSuffix(f, "%")
				if cov, err := strconv.ParseFloat(f, 64); err == nil {
					result.Coverage = cov
					result.CoverageStr = fmt.Sprintf("%.1f%%", cov)
				}
			}
		}
	}

	results[pkg] = result
}

// printPackageSummary prints the package coverage summary table
// Returns true if any package is below threshold (CRITICAL)
func printPackageSummary(results []PackageResult, testCounts map[string]int) bool {
	hasCritical := false

	fmt.Println("Package coverage summary:")
	if cfg.ShowTestCounts {
		fmt.Printf("%-3s %-40s %-10s %-10s %s\n", "OK", "PACKAGE", "STATEMENTS", "TESTS", "COVERAGE")
	} else {
		fmt.Printf("%-3s %-40s %-10s %-10s %s\n", "OK", "PACKAGE", "STATEMENTS", "", "COVERAGE")
	}
	fmt.Println(strings.Repeat("-", 80))

	// Sort by coverage descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Coverage > results[j].Coverage
	})

	for _, r := range results {
		if r.Status == "ok" {
			duration := r.Duration
			if r.Cached {
				duration = "(cached)"
			}

			tests := testCounts[r.Name]
			coverageStr := r.CoverageStr
			isCritical := r.Coverage < cfg.ThresholdPackage

			if isCritical {
				hasCritical = true
				if cfg.CIMode {
					fmt.Printf("::error::%-3s %-40s %-10s %-10s %s (CRITICAL: < %.0f%%)\n",
						r.Status, r.Name, duration, "", coverageStr, cfg.ThresholdPackage)
				} else {
					coverageStr = fmt.Sprintf("%s%s%s (CRITICAL: < %.0f%%)", ColorRed, r.CoverageStr, ColorReset, cfg.ThresholdPackage)
					fmt.Printf("%-3s %-40s %-10s %-10d %s\n", r.Status, r.Name, duration, tests, coverageStr)
				}
			} else {
				if cfg.ShowTestCounts {
					fmt.Printf("%-3s %-40s %-10s %-10d %s\n", r.Status, r.Name, duration, tests, coverageStr)
				} else {
					fmt.Printf("%-3s %-40s %-10s %-10s %s\n", r.Status, r.Name, duration, "", coverageStr)
				}
			}
		}
	}

	// Print skipped packages
	for _, r := range results {
		if r.Status == "?" {
			fmt.Printf("%-3s %-40s %s\n", r.Status, r.Name, "[no test files]")
		}
	}

	return hasCritical
}

// getFunctionCoverage runs go tool cover -func and parses output
func getFunctionCoverage() []FuncCoverage {
	cmd := exec.Command("go", "tool", "cover", "-func="+cfg.CoverProfile)
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting function coverage: %v\n", err)
		return nil
	}

	var funcs []FuncCoverage
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimPrefix(line, ModulePrefix)

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		if fields[0] == "total:" {
			continue
		}

		covStr := strings.TrimSuffix(fields[len(fields)-1], "%")
		cov, _ := strconv.ParseFloat(covStr, 64)

		funcs = append(funcs, FuncCoverage{
			Location: fields[0],
			Function: fields[1],
			Coverage: cov,
		})
	}

	return funcs
}

// printFunctionCoverage prints function coverage details
// Returns true if any function is below threshold (CRITICAL)
func printFunctionCoverage(funcs []FuncCoverage) bool {
	hasCritical := false

	fmt.Printf("\nFunction coverage details (excluding >= %.0f%%):\n", cfg.ThresholdPrint)
	fmt.Printf("%-70s %-35s %s\n", "LOCATION", "FUNCTION", "COVERAGE")
	fmt.Println(strings.Repeat("-", 105))

	// Count functions above threshold
	aboveThreshold := 0
	var belowThreshold []FuncCoverage

	for _, f := range funcs {
		if f.Coverage >= cfg.ThresholdPrint {
			aboveThreshold++
		} else {
			belowThreshold = append(belowThreshold, f)
		}
	}

	fmt.Printf("... %d more...\n", aboveThreshold)

	// Sort by coverage descending
	sort.Slice(belowThreshold, func(i, j int) bool {
		return belowThreshold[i].Coverage > belowThreshold[j].Coverage
	})

	for _, f := range belowThreshold {
		isCritical := f.Coverage < cfg.ThresholdFunc
		if isCritical {
			hasCritical = true
		}

		covStr := fmt.Sprintf("%.1f%%", f.Coverage)
		if isCritical {
			if cfg.CIMode {
				// Parse file and line from location
				parts := strings.Split(f.Location, ":")
				file := parts[0]
				line := "1"
				if len(parts) > 1 {
					line = parts[1]
				}
				fmt.Printf("::error file=%s,line=%s::%-70s %-35s %s (CRITICAL < %.0f%%)\n",
					file, line, f.Location, f.Function, covStr, cfg.ThresholdFunc)
			} else {
				covStr = fmt.Sprintf("%s%.1f%%%s (CRITICAL: < %.0f%%)", ColorRed, f.Coverage, ColorReset, cfg.ThresholdFunc)
				fmt.Printf("%-70s %-35s %s\n", f.Location, f.Function, covStr)
			}
		} else {
			fmt.Printf("%-70s %-35s %s\n", f.Location, f.Function, covStr)
		}
	}

	fmt.Println(strings.Repeat("-", 105))
	return hasCritical
}

// printTotal prints total coverage and checks threshold
// Returns true if total is below threshold (CRITICAL)
func printTotal() bool {
	totalCov := getTotalCoverage()
	isCritical := totalCov < cfg.ThresholdTotal

	totalStr := fmt.Sprintf("%.1f%%", totalCov)
	if isCritical {
		if cfg.CIMode {
			fmt.Printf("::error::%-96s %s (CRITICAL: < %.0f%%)\n", "TOTAL", totalStr, cfg.ThresholdTotal)
		} else {
			totalStr = fmt.Sprintf("%s%.1f%%%s (CRITICAL: < %.0f%%)", ColorRed, totalCov, ColorReset, cfg.ThresholdTotal)
			fmt.Printf("%-96s %s\n", "TOTAL", totalStr)
		}
	} else {
		fmt.Printf("%-96s %s\n", "TOTAL", totalStr)
	}
	fmt.Println(strings.Repeat("-", 105))

	return isCritical
}

// getTotalCoverage calculates total coverage from coverage profile
func getTotalCoverage() float64 {
	cmd := exec.Command("go", "tool", "cover", "-func="+cfg.CoverProfile)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "total:") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				covStr := strings.TrimSuffix(fields[len(fields)-1], "%")
				cov, _ := strconv.ParseFloat(covStr, 64)
				return cov
			}
		}
	}
	return 0
}

// printStatistics prints coverage statistics
func printStatistics(funcs []FuncCoverage, testCounts map[string]int) {
	count100 := 0
	count95_100 := 0
	count85_95 := 0
	countLt85 := 0

	for _, f := range funcs {
		switch {
		case f.Coverage == 100.0:
			count100++
		case f.Coverage >= 95.0:
			count95_100++
		case f.Coverage >= 85.0:
			count85_95++
		default:
			countLt85++
		}
	}

	fmt.Println("Statistics:")
	fmt.Printf("Functions with 100%% coverage: %d\n", count100)
	fmt.Printf("Functions with 95%%-100%% coverage: %d\n", count95_100)
	fmt.Printf("Functions with 85%%-95%% coverage: %d\n", count85_95)
	fmt.Printf("Functions with <85%% coverage: %d\n", countLt85)

	// Count total tests
	if cfg.ShowTestCounts {
		totalTopLevel := 0
		for _, count := range testCounts {
			totalTopLevel += count
		}
		fmt.Printf("Total tests: %d top-level\n", totalTopLevel)
	}
}

// generateHTMLReport generates an HTML coverage report
func generateHTMLReport() {
	cmd := exec.Command("go", "tool", "cover", "-html="+cfg.CoverProfile, "-o", "test_coverage.html")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating HTML report: %v\n", err)
	}
}

// analyzeUncoveredBlocks calls the existing uncovered_blocks tool
// Returns true if any CRITICAL blocks found
func analyzeUncoveredBlocks() bool {
	cmd := exec.Command("go", "run", "./scripts/lib/uncovered_blocks/",
		cfg.CoverProfile, "false", strconv.Itoa(cfg.UncoveredLimit))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err != nil
}
