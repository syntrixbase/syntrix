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

// Thresholds for coverage analysis
const (
	ThresholdFunc    = 80.0
	ThresholdPackage = 85.0
	ThresholdPrint   = 85.0
	ThresholdTotal   = 90.0

	ModulePrefix = "github.com/syntrixbase/syntrix/"
)

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

func main() {
	coverProfile := os.Getenv("COVERPROFILE")
	if coverProfile == "" {
		coverProfile = "/tmp/coverage.out"
	}

	pkgs := "./internal/... ./tests/... ./pkg/..."

	fmt.Println()

	// Run tests and collect results
	results, testCounts, exitCode := runTests(pkgs, coverProfile)
	if exitCode != 0 && len(results) == 0 {
		os.Exit(exitCode)
	}

	// Print package coverage summary
	printPackageSummary(results, testCounts)

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	// Get function coverage data
	funcData := getFunctionCoverage(coverProfile)

	// Print function coverage details
	printFunctionCoverage(funcData)

	// Print statistics
	printStatistics(funcData, testCounts)

	// Generate HTML report
	generateHTMLReport(coverProfile)

	// Analyze uncovered blocks
	fmt.Println()
	fmt.Println(strings.Repeat("-", 90))
	analyzeUncoveredBlocks(coverProfile)
}

// runTests executes go test and parses JSON output
func runTests(pkgs, coverProfile string) ([]PackageResult, map[string]int, int) {
	args := []string{"test"}
	args = append(args, strings.Fields(pkgs)...)
	args = append(args, "-json", "-covermode=atomic", "-coverprofile="+coverProfile)

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
func printPackageSummary(results []PackageResult, testCounts map[string]int) {
	fmt.Println("Package coverage summary:")
	fmt.Printf("%-3s %-40s %-10s %-10s %s\n", "OK", "PACKAGE", "STATEMENTS", "TESTS", "COVERAGE")
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

			if r.Coverage < ThresholdPackage {
				coverageStr = fmt.Sprintf("%s%s%s (CRITICAL: < %.0f%%)", ColorRed, r.CoverageStr, ColorReset, ThresholdPackage)
			}

			fmt.Printf("%-3s %-40s %-10s %-10d %s\n", r.Status, r.Name, duration, tests, coverageStr)
		}
	}

	// Print skipped packages
	for _, r := range results {
		if r.Status == "?" {
			fmt.Printf("%-3s %-40s %s\n", r.Status, r.Name, "[no test files]")
		}
	}
}

// getFunctionCoverage runs go tool cover -func and parses output
func getFunctionCoverage(coverProfile string) []FuncCoverage {
	cmd := exec.Command("go", "tool", "cover", "-func="+coverProfile)
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
func printFunctionCoverage(funcs []FuncCoverage) {
	fmt.Printf("\nFunction coverage details (excluding >= %.0f%%):\n", ThresholdPrint)
	fmt.Printf("%-60s %-35s %s\n", "LOCATION", "FUNCTION", "COVERAGE")
	fmt.Println(strings.Repeat("-", 105))

	// Count functions above threshold
	aboveThreshold := 0
	var belowThreshold []FuncCoverage

	for _, f := range funcs {
		if f.Coverage >= ThresholdPrint {
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
		covStr := fmt.Sprintf("%.1f%%", f.Coverage)
		if f.Coverage < ThresholdFunc {
			covStr = fmt.Sprintf("%s%.1f%%%s (CRITICAL: < %.0f%%)", ColorRed, f.Coverage, ColorReset, ThresholdFunc)
		}
		fmt.Printf("%-60s %-35s %s\n", f.Location, f.Function, covStr)
	}

	fmt.Println(strings.Repeat("-", 105))

	// Print total
	totalCov := getTotalCoverage(funcs)
	totalStr := fmt.Sprintf("%.1f%%", totalCov)
	if totalCov < ThresholdTotal {
		totalStr = fmt.Sprintf("%s%.1f%%%s (CRITICAL: < %.0f%%)", ColorRed, totalCov, ColorReset, ThresholdTotal)
	}
	fmt.Printf("%-96s %s\n", "TOTAL", totalStr)
	fmt.Println(strings.Repeat("-", 105))
}

// getTotalCoverage calculates total coverage from function data
func getTotalCoverage(funcs []FuncCoverage) float64 {
	// Re-run go tool cover to get total
	cmd := exec.Command("go", "tool", "cover", "-func=/tmp/coverage.out")
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
	totalTopLevel := 0
	for _, count := range testCounts {
		totalTopLevel += count
	}
	fmt.Printf("Total tests: %d top-level\n", totalTopLevel)
}

// generateHTMLReport generates an HTML coverage report
func generateHTMLReport(coverProfile string) {
	cmd := exec.Command("go", "tool", "cover", "-html="+coverProfile, "-o", "test_coverage.html")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating HTML report: %v\n", err)
	}
}

// analyzeUncoveredBlocks calls the existing uncovered_blocks tool
func analyzeUncoveredBlocks(coverProfile string) {
	cmd := exec.Command("go", "run", "./scripts/lib/uncovered_blocks/", coverProfile, "false", "10")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
