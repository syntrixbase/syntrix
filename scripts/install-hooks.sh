#!/bin/bash
set -e

# Get the git directory (usually .git)
GIT_DIR=$(git rev-parse --git-dir)
HOOKS_DIR="$GIT_DIR/hooks"
SCRIPT_DIR=$(dirname "$0")
# The hook source is now in scripts/hooks/gofmt-pre-commit.sh
PRE_COMMIT_SOURCE="$SCRIPT_DIR/hooks/gofmt-pre-commit.sh"

# Resolve absolute path of the pre-commit source
ABS_PRE_COMMIT_SOURCE=$(realpath "$PRE_COMMIT_SOURCE")

echo "Installing pre-commit hook..."

# Create the hook file (symlink is better if supported, but a wrapper script is safer across environments)
# We will use a wrapper script that calls the source script.
cat > "$HOOKS_DIR/pre-commit" <<EOF
#!/bin/bash
"$ABS_PRE_COMMIT_SOURCE"
EOF

chmod +x "$HOOKS_DIR/pre-commit"

echo "Pre-commit hook installed successfully at $HOOKS_DIR/pre-commit"
