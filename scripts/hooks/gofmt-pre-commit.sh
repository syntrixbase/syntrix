#!/bin/bash
set -e

# Find all staged .go files
# --diff-filter=ACM ensures we only look at Added, Copied, or Modified files (not Deleted)
STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep "\.go$" || true)

if [ -z "$STAGED_GO_FILES" ]; then
    exit 0
fi

echo "Running gofmt on staged Go files..."

# Format each file and re-add it to the commit
for FILE in $STAGED_GO_FILES; do
    if [ -f "$FILE" ]; then
        gofmt -w "$FILE"
        git add "$FILE"
    fi
done

echo "Go files formatted and re-staged."
