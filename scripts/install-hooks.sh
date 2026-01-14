#!/bin/bash
set -e

cd $(dirname $0)/../

# Get the git directory (usually .git)
GIT_DIR=$(git rev-parse --git-dir)
HOOKS_DIR="$GIT_DIR/hooks"
SCRIPT_DIR=$(dirname "$0")

# Hook sources
PRE_COMMIT_SOURCE="$SCRIPT_DIR/hooks/gofmt-pre-commit.sh"
COMMIT_MSG_SOURCE="$SCRIPT_DIR/hooks/commit-msg.sh"

# Resolve absolute paths
ABS_PRE_COMMIT_SOURCE=$(realpath "$PRE_COMMIT_SOURCE")
ABS_COMMIT_MSG_SOURCE=$(realpath "$COMMIT_MSG_SOURCE")

# Install pre-commit hook
echo "Installing pre-commit hook..."
cat > "$HOOKS_DIR/pre-commit" <<EOF
#!/bin/bash
"$ABS_PRE_COMMIT_SOURCE"
EOF
chmod +x "$HOOKS_DIR/pre-commit"
echo "Pre-commit hook installed successfully at $HOOKS_DIR/pre-commit"

# Install commit-msg hook
echo "Installing commit-msg hook..."
cat > "$HOOKS_DIR/commit-msg" <<EOF
#!/bin/bash
"$ABS_COMMIT_MSG_SOURCE" "\$1"
EOF
chmod +x "$HOOKS_DIR/commit-msg"
echo "Commit-msg hook installed successfully at $HOOKS_DIR/commit-msg"
