package main

import (
	"flag"
	"os"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	// Save original args and flags
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	defer flag.CommandLine.Init("test", flag.ContinueOnError)

	// Reset flag.CommandLine for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Set args to trigger version flag
	os.Args = []string{"cmd", "-version"}

	// Reinitialize flags
	testVersion := flag.Bool("version", false, "Print version and exit")
	_ = flag.Bool("debug", false, "Enable debug logging")

	// Parse flags
	flag.Parse()

	if !*testVersion {
		t.Error("Expected version flag to be true")
	}
}

func TestDebugFlag(t *testing.T) {
	// Reset flag.CommandLine for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Reinitialize flags
	_ = flag.Bool("version", false, "Print version and exit")
	testDebug := flag.Bool("debug", false, "Enable debug logging")

	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set args to trigger debug flag
	os.Args = []string{"cmd", "-debug"}

	// Parse flags
	flag.Parse()

	if !*testDebug {
		t.Error("Expected debug flag to be true")
	}
}

func TestDefaultFlags(t *testing.T) {
	// Reset flag.CommandLine for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Reinitialize flags
	testVersion := flag.Bool("version", false, "Print version and exit")
	testDebug := flag.Bool("debug", false, "Enable debug logging")

	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set args with no flags
	os.Args = []string{"cmd"}

	// Parse flags
	flag.Parse()

	if *testVersion {
		t.Error("Expected version flag to be false by default")
	}
	if *testDebug {
		t.Error("Expected debug flag to be false by default")
	}
}

func TestConstants(t *testing.T) {
	if defaultSessionMaxAge.Minutes() != 30 {
		t.Errorf("Expected defaultSessionMaxAge to be 30 minutes, got %v", defaultSessionMaxAge.Minutes())
	}
	if cleanupInterval.Minutes() != 5 {
		t.Errorf("Expected cleanupInterval to be 5 minutes, got %v", cleanupInterval.Minutes())
	}
}
