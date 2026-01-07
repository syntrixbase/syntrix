#!/bin/bash
set -e

# Get the workspace root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(dirname "$SCRIPT_DIR")"

# Detect Go version from go.mod
GO_VERSION=$(grep "^go " "$WORKSPACE_ROOT/go.mod" | awk '{print $2}')
if [ -z "$GO_VERSION" ]; then
    GO_VERSION="latest"
fi

IMAGE="golang:${GO_VERSION}"

echo "Starting Docker container with 2 CPUs (Image: $IMAGE)..."
echo "Command: $@"

# Run the command in a disposable container
# - --rm: Remove container after exit
# - --cpus="2": Limit to 2 CPU cores
# - -v: Mount the workspace
# - -w: Set working directory to workspace
# - -e: Pass environment variables if needed (e.g. CGO_ENABLED)
# - --net=host: Allow accessing host services (db, etc) if needed by tests

docker run --rm \
    --cpus="1" \
    --net=host \
    -v "$WORKSPACE_ROOT:/workspace" \
    -v "${GOPATH:-$HOME/go}/pkg/mod:/go/pkg/mod" \
    -v "${GOCACHE:-$HOME/.cache/go-build}:/root/.cache/go-build" \
    -w "/workspace" \
    "$IMAGE" \
    "$@"
