package main

import (
	"fmt"
	"os"
)

// Version information
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Fprintf(os.Stdout, "Syntrix Benchmark Tool v%s\n", Version)
	fmt.Fprintf(os.Stdout, "Git Commit: %s\n", GitCommit)
	fmt.Fprintf(os.Stdout, "Build Date: %s\n\n", BuildDate)

	fmt.Fprintln(os.Stdout, "Usage: syntrix-benchmark [command] [flags]")
	fmt.Fprintln(os.Stdout, "Commands:")
	fmt.Fprintln(os.Stdout, "  run       Run a benchmark scenario")
	fmt.Fprintln(os.Stdout, "  compare   Compare benchmark results (coming soon)")
	fmt.Fprintln(os.Stdout, "  version   Show version information")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Run 'syntrix-benchmark [command] --help' for more information about a command.")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Note: Full CLI implementation coming soon. See tests/benchmark/TASKS.md for progress.")
}
