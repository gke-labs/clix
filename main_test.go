package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	// Create a temporary script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-script")

	// The script points to our local test-tool
	// We need absolute path to test-tool for it to work reliably from anywhere
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}
	testToolPath := filepath.Join(cwd, "tests", "test-tool")

	scriptContent := fmt.Sprintf(`#!/usr/bin/env clix
go:
  run: %s
`, testToolPath)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")

	args := []string{"clix", scriptPath, "foo", "bar"}

	err = run(stdin, &stdout, &stderr, args)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	expectedOutput := "Hello from test-tool"
	if !strings.Contains(output, expectedOutput) {
		t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
	}
	
	if !strings.Contains(output, "Arg 0: foo") {
		t.Errorf("Expected output to contain 'Arg 0: foo', got %q", output)
	}
}
