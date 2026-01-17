#!/usr/bin/env bash
# Generate Go test coverage report.
# Usage: ./scripts/coverage.sh [html|func]
#   html: (default) Generate HTML report and print summary.
#   func: Print function-level coverage details.

cd $(dirname $0)/../

set -euo pipefail

MODE=${1:-html}

# Optional output path override: COVERPROFILE=/path/to/coverage.out ./scripts/coverage.sh
COVERPROFILE=${COVERPROFILE:-"/tmp/coverage.out"}

PKGS="./internal/... ./tests/... ./pkg/..."

threshold_func=80.0
threshold_package=85.0
threshold_print=85.0
threshold_total=90.0

# Use temp files to capture output
TMP_OUTPUT=$(mktemp)
TMP_JSON=$(mktemp)
trap 'rm -f "$TMP_OUTPUT" "$TMP_JSON"' EXIT

# Run tests with -json for both coverage and test counting in one pass
set +e
go test ${PKGS} -json -covermode=atomic -coverprofile="$COVERPROFILE" 2>&1 | tee "$TMP_JSON" | \
    jq -r 'select(.Action == "output") | .Output // empty' | grep -E '^(ok|FAIL|\?)' > "$TMP_OUTPUT"
EXIT_CODE=${PIPESTATUS[0]}
set -e

sed -i 's/of statements//g; s/github.com\/syntrixbase\/syntrix\///g' "$TMP_OUTPUT"

# Count test cases per package and save to temp file
TMP_TEST_COUNTS=$(mktemp)
trap 'rm -f "$TMP_OUTPUT" "$TMP_JSON" "$TMP_TEST_COUNTS"' EXIT
jq -r 'select(.Action == "run") | select(.Test) | select(.Test | contains("/") | not) | .Package' "$TMP_JSON" 2>/dev/null | \
    sed 's|github.com/syntrixbase/syntrix/||' | \
    sort | uniq -c | awk '{print $2, $1}' > "$TMP_TEST_COUNTS"

echo
echo "Package coverage summary:"
printf "%-3s %-40s %-10s %-10s %s\n" "OK" "PACKAGE" "STATEMENTS" "TESTS" "COVERAGE"
echo "--------------------------------------------------------------------------------"
# Process and sort 'ok' lines (coverage data)
grep "^ok" "$TMP_OUTPUT" | \
    grep -vE "^ok\s+tests/" | \
    while read -r line; do
        pkg=$(echo "$line" | awk '{print $2}')
        test_count=$(grep "^$pkg " "$TMP_TEST_COUNTS" | awk '{print $2}')
        test_count=${test_count:-0}
        echo "$line $test_count"
    done | \
    awk -v threshold_package="$threshold_package" '{
        cov = $5;
        sub("%", "", cov);
        tests = $6;
        if (cov + 0 < threshold_package + 0) {
            printf "%-3s %-40s %-10s %-10s \033[31m%s\033[0m (CRITICAL: < %s%%)\n", $1, $2, $3, tests, $5, threshold_package
        } else {
            printf "%-3s %-40s %-10s %-10s %s\n", $1, $2, $3, tests, $5
        }
    }' | \
    sort -k5 -nr || true

# Process and print other lines (skipped, failures, etc.)
grep -v "^ok" "$TMP_OUTPUT" | \
    awk '{
        if ($1 == "?") {
             printf "%-3s %-40s %s %s %s\n", $1, $2, $3, $4, $5
        } else {
            print $0
        }
    }' || true

if [ $EXIT_CODE -ne 0 ]; then
    exit $EXIT_CODE
fi

echo -e "\nFunction coverage details (excluding >= ${threshold_print}%):"

FUNC_DATA=$(go tool cover -func="$COVERPROFILE" | sed 's/github.com\/syntrixbase\/syntrix\///g')

printf "%-60s %-35s %s\n" "LOCATION" "FUNCTION" "COVERAGE"
echo "---------------------------------------------------------------------------------------------------------"
echo "$FUNC_DATA" | \
    grep -v "^total:" | \
    awk -v threshold_print="$threshold_print" 'BEGIN {total=0}{
        cov = $3;sub("%", "", cov);
        if (cov + 0 >= threshold_print + 0) {
            total += 1
        }
    } END{print "... " total " more..."}'
# Print functions with < 85% coverage
echo "$FUNC_DATA" | \
    grep -v "^total:" | \
    awk -v threshold_print="$threshold_print" -v threshold_func="$threshold_func" '{
        cov = $3;
        sub("%", "", cov);
        if (cov + 0 < threshold_func + 0) {
            # Mark functions below function threshold with *
            printf "%-60s %-35s \033[31m%s\033[0m (CRITICAL: < %s%%)\n", $1, $2, $3, threshold_func
        } else if (cov + 0 < threshold_print + 0) {
            printf "%-60s %-35s %s\n", $1, $2, $3
        }
    }' | \
    sort -k3 -nr

echo "---------------------------------------------------------------------------------------------------------"

echo "$FUNC_DATA" | \
    grep "^total:" | \
    awk -v threshold_total="$threshold_total" '{
        if ($3 + 0 < threshold_total + 0) {
            # Mark total below threshold with *
            printf "%-96s \033[31m%s\033[0m (CRITICAL: < %s%%)\n", "TOTAL", $3, threshold_total
        } else {
            printf "%-96s %s\n", "TOTAL", $3
        }
    }'

echo "---------------------------------------------------------------------------------------------------------"

COUNT_100=$(echo "$FUNC_DATA" | awk '$3 == "100.0%"' | wc -l)
COUNT_95_100=$(echo "$FUNC_DATA" | awk '{cov=$3; sub("%", "", cov); if (cov + 0 >= 95.0 && cov + 0 < 100.0) print $0}' | wc -l)
COUNT_85_95=$(echo "$FUNC_DATA" | awk '{cov=$3; sub("%", "", cov); if (cov + 0 >= 85.0 && cov + 0 < 95.0) print $0}' | wc -l)
COUNT_LE_85=$(echo "$FUNC_DATA" | awk '{cov=$3; sub("%", "", cov); if (cov + 0 < 85.0) print $0}' | wc -l)

echo "Statistics:"
echo "Functions with 100% coverage: $COUNT_100"
echo "Functions with 95%-100% coverage: $COUNT_95_100"
echo "Functions with 85%-95% coverage: $COUNT_85_95"
echo "Functions with <85% coverage: $COUNT_LE_85"

# Count tests from the JSON output (already captured, no need to re-run)
TOP_LEVEL_TESTS=$(jq -s '[.[] | select(.Action == "run") | select(.Test != null) | select(.Test | contains("/") | not)] | length' "$TMP_JSON" 2>/dev/null || echo 0)
TOTAL_TESTS=$(jq -s '[.[] | select(.Action == "pass") | select(.Test != null)] | length' "$TMP_JSON" 2>/dev/null || echo 0)
echo "Total tests: $TOTAL_TESTS (including subtests, $TOP_LEVEL_TESTS top-level)"

# Generate HTML report or print function details based on mode
go tool cover -html=$COVERPROFILE -o test_coverage.html

echo ""
echo "---------------------------------------------------------------------------------------------------------"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
go run "$SCRIPT_DIR/lib/uncovered_blocks/" "$COVERPROFILE" "false" 10
