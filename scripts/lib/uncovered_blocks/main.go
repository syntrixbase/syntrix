package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

var maxOutputBlocks = 40

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./ <coverage_file> [print_description or not] [print_line_count]")
		os.Exit(1)
	}

	coverFile := os.Args[1]
	printDescription := os.Args[2] == "true" // Any extra args
	printLineCount := os.Args[3]
	file, err := os.Open(coverFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	if n, err := strconv.Atoi(printLineCount); err == nil {
		if n > 0 && n < 500 {
			maxOutputBlocks = n
		}
	}

	var blocks []Block
	scanner := bufio.NewScanner(file)

	// Skip mode line
	if scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "mode:") {
			// If not mode line, process it (though usually first line is mode)
			if b, ok := parseLine(line); ok {
				blocks = append(blocks, b)
			}
		}
	}

	for scanner.Scan() {
		if b, ok := parseLine(scanner.Text()); ok {
			blocks = append(blocks, b)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	merged := mergeBlocks(blocks)

	// Analyze blocks using AST
	fileCache := NewFileCache()
	astCache := NewASTCache()
	for i := range merged {
		AnalyzeBlockWithAST(&merged[i], astCache, fileCache)
	}

	// Sort by Level (CRITICAL > HIGH > MEDIUM > LOW) then by NumLines descending
	levelWeight := map[string]int{
		"CRITICAL": 4,
		"HIGH":     3,
		"MEDIUM":   2,
		"LOW":      1,
	}

	sort.Slice(merged, func(i, j int) bool {
		w1 := levelWeight[merged[i].Level]
		w2 := levelWeight[merged[j].Level]
		if w1 != w2 {
			return w1 > w2
		}
		if merged[i].EffectiveLines != merged[j].EffectiveLines {
			return merged[i].EffectiveLines > merged[j].EffectiveLines
		}
		return merged[i].NumLines > merged[j].NumLines
	})

	// Calculate max location width and print
	maxLocWidth := calculateMaxLocWidth(merged)
	printHeader(maxLocWidth, printDescription)
	printBlocks(merged, maxLocWidth)
}
