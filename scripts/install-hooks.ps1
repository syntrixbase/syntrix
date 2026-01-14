$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

$gitDir = (git rev-parse --git-dir) 2>$null
if (-not $gitDir) {
    Write-Error 'Not a git repository.'
    exit 1
}

$hooksDir = Join-Path $gitDir 'hooks'
New-Item -ItemType Directory -Force -Path $hooksDir | Out-Null

# Install pre-commit hook
$preCommitPath = Join-Path $hooksDir 'pre-commit'
$preCommitScript = @(
    '#!/bin/sh',
    'REPO_ROOT="$(git rev-parse --show-toplevel)"',
    'HOOK_SCRIPT="$REPO_ROOT/scripts/hooks/gofmt-pre-commit.sh"',
    'exec "$HOOK_SCRIPT"'
) -join "`n"
[System.IO.File]::WriteAllText($preCommitPath, $preCommitScript, [System.Text.Encoding]::ASCII)
Write-Output "Pre-commit hook installed successfully at $preCommitPath"

# Install commit-msg hook
$commitMsgPath = Join-Path $hooksDir 'commit-msg'
$commitMsgScript = @(
    '#!/bin/sh',
    'REPO_ROOT="$(git rev-parse --show-toplevel)"',
    'HOOK_SCRIPT="$REPO_ROOT/scripts/hooks/commit-msg.sh"',
    'exec "$HOOK_SCRIPT" "$1"'
) -join "`n"
[System.IO.File]::WriteAllText($commitMsgPath, $commitMsgScript, [System.Text.Encoding]::ASCII)
Write-Output "Commit-msg hook installed successfully at $commitMsgPath"
