@echo off
setlocal

:: Optional output path override
set COVERPROFILE=coverage.out
if not "%~1"=="" set COVERPROFILE=%~1

echo Running go test with coverage...
echo Excluding packages matching: /cmd/

:: Use PowerShell to list packages and filter out cmd packages
for /f "usebackq tokens=*" %%i in (`powershell -NoProfile -Command "(go list ./... | Where-Object { $_ -notmatch '/cmd/' }) -join ' '"`) do set PKGS=%%i

if "%PKGS%"=="" (
    echo No packages found to test.
    exit /b 1
)

:: Run tests
go test %PKGS% -covermode=atomic -coverprofile="%COVERPROFILE%" 2>&1 | powershell -NoProfile -Command "$input | ForEach-Object { $_ -replace ' of statements', '' }"

echo.
echo Coverage summary:
go tool cover -func="%COVERPROFILE%" | findstr "total:" | powershell -NoProfile -Command "$input | ForEach-Object { $_ -replace '\s+', ' ' -replace '\(statements\)\s*', '' }"

go tool cover -html="%COVERPROFILE%" -o test_coverage.html
echo To view HTML report: go tool cover -html="%COVERPROFILE%"

endlocal
