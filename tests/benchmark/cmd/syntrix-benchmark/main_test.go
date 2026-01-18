package main

import (
	"testing"
)

func TestMain(t *testing.T) {
	// Basic sanity test
	t.Log("Benchmark tool main package test")
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
}
