@echo off
setlocal

:: Mode: html (default) or func
set MODE=%1
if "%MODE%"=="" set MODE=html

:: COVERPROFILE can be overridden via env COVERPROFILE
if "%COVERPROFILE%"=="" (
    set COVERPROFILE=coverage.out
)

set EXCLUDE_REGEX=/cmd/

:: Build package list excluding cmd/
for /f "usebackq tokens=*" %%i in (`powershell -NoProfile -Command "(go list ./... | Where-Object { $_ -notmatch '%EXCLUDE_REGEX%' }) -join ' '"`) do set PKGS=%%i

if "%PKGS%"=="" (
    echo No packages found to test.
    exit /b 1
)

echo Excluding packages matching: %EXCLUDE_REGEX%

:: Temp file for capturing output
for /f "usebackq" %%i in (`powershell -NoProfile -Command "New-TemporaryFile | Select-Object -ExpandProperty FullName"`) do set TMPFILE=%%i

:: Run go test with coverage and capture output
go test %PKGS% -covermode=atomic -coverprofile="%COVERPROFILE%" >"%TMPFILE%" 2>&1
set EXITCODE=%ERRORLEVEL%

:: Process ok lines: strip module prefix and sort by coverage descending (column 5)
powershell -NoProfile -Command " & { $content = Get-Content '%TMPFILE%'; $ok = $content | Where-Object { $_ -match '^ok' } | ForEach-Object { $_ -replace 'of statements','' -replace 'github.com/codetrek/syntrix/','' }; $ok | Sort-Object { ($_ -split '\s+')[4] } -Descending | ForEach-Object { $_ }; $other = $content | Where-Object { $_ -notmatch '^ok' } | ForEach-Object { $_ -replace 'github.com/codetrek/syntrix/','' }; $other | ForEach-Object { $_ }; if (%EXITCODE% -ne 0) { exit %EXITCODE% }; if ('%MODE%' -ieq 'func') { Write-Output ''; Write-Output 'Function coverage details (excluding 100%%):'; Write-Output ('{0,-60} {1,-35} {2}' -f 'LOCATION', 'FUNCTION', 'COVERAGE'); Write-Output ('-'*97); $funcData = go tool cover -func='%COVERPROFILE%' | ForEach-Object { $_ -replace 'github.com/codetrek/syntrix/','' }; $funcData | Where-Object { $_ -notmatch '^total:' } | ForEach-Object { $parts = $_ -split '\s+'; if ($parts.Count -ge 3) { $cov = $parts[-1]; if ($cov -ne '100.0%%') { $loc = $parts[0]; $fn = $parts[1]; ('{0,-60} {1,-35} {2}' -f $loc, $fn, $cov) } } } | Sort-Object { ($_ -split '\s+')[-1].TrimEnd('%%') } -Descending | ForEach-Object { $_ }; Write-Output ('-'*97); $count = ($funcData | Where-Object { $_ -match '100.0%%' }).Count; Write-Output ('Functions with 100%% coverage: ' + $count); $funcData | Where-Object { $_ -match '^total:' } | ForEach-Object { $parts = $_ -split '\s+'; ('{0,-96} {1}' -f 'TOTAL', $parts[-1]) }; } else { Write-Output ''; Write-Output 'Coverage summary:'; go tool cover -func='%COVERPROFILE%' | Select-Object -Last 1 | ForEach-Object { $parts = $_ -split '\s+'; ('{0,-10} {1,-15} {2}' -f $parts[0], $parts[1], $parts[2]) }; go tool cover -html='%COVERPROFILE%' -o test_coverage.html; Write-Output ''; Write-Output ('To view HTML report: go tool cover -html=%COVERPROFILE%'); } } "

del "%TMPFILE%"

endlocal
