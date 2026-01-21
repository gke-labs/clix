// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

func TestRunDocker(t *testing.T) {
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker not found")
	}

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-script-docker")

	scriptContent := `#!/usr/bin/env clix
image: alpine
entrypoint: echo
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")

	// We use 'echo' as entrypoint and pass 'hello' as arg.
	// Expected output: hello
	args := []string{"clix", scriptPath, "hello"}

	err = run(stdin, &stdout, &stderr, args)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("Expected output to contain 'hello', got %q", output)
	}
}

func TestResolveMounts(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get user home: %v", err)
	}

	tests := []struct {
		name     string
		input    []Mount
		expected []Mount
	}{
		{
			name: "Home directory expansion",
			input: []Mount{
				{HostPath: "~/.config"},
				{HostPath: "~/data"},
				{HostPath: "~"},
			},
			expected: []Mount{
				{HostPath: filepath.Join(home, ".config"), SandboxPath: filepath.Join(home, ".config")},
				{HostPath: filepath.Join(home, "data"), SandboxPath: filepath.Join(home, "data")},
				{HostPath: home, SandboxPath: home},
			},
		},
		{
			name: "No expansion",
			input: []Mount{
				{HostPath: "/absolute/path"},
			},
			expected: []Mount{
				{HostPath: "/absolute/path", SandboxPath: "/absolute/path"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMounts(tt.input)
			if err != nil {
				t.Fatalf("resolveMounts failed: %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d mounts, got %d", len(tt.expected), len(got))
			}

			for i, m := range got {
				if m.HostPath != tt.expected[i].HostPath {
					t.Errorf("mount[%d].HostPath = %q, want %q", i, m.HostPath, tt.expected[i].HostPath)
				}
				if m.SandboxPath != tt.expected[i].SandboxPath {
					t.Errorf("mount[%d].SandboxPath = %q, want %q", i, m.SandboxPath, tt.expected[i].SandboxPath)
				}
			}
		})
	}
}
