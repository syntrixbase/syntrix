#!/bin/bash
set -e

COMMIT_MSG_FILE="$1"
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

if echo "$COMMIT_MSG" | grep -qi "co-authored-by:"; then
    echo "❌ Commit rejected: commit message contains 'co-author' which is forbidden."
    echo "   Please remove any Co-Authored-By lines from your commit message."
    exit 1
fi

if echo "$COMMIT_MSG" | grep -qi "generated with ["; then
    echo "❌ Commit rejected: commit message contains 'Generated with' which is forbidden."
    echo "   Please remove any 'Generated with' lines from your commit message."
    exit 1
fi

exit 0
