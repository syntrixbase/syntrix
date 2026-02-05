$ErrorActionPreference = 'Stop'

$scriptDir = $PSScriptRoot
$projectRoot = Split-Path $scriptDir -Parent
$hooksDir = Join-Path $projectRoot 'git-hooks'

# Set git to use our hooks directory
Write-Output "Setting git hooks path to $hooksDir..."
git config core.hooksPath $hooksDir

Write-Output "Git hooks configured successfully!"
Write-Output "Hooks path: $hooksDir"
Write-Output ""
Write-Output "Active hooks:"
Write-Output "  - pre-commit: Block main branch commits, run gofmt"
Write-Output "  - commit-msg: Block co-author credits"
Write-Output "  - pre-push: Run coverage check"
