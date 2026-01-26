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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunShfmt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test in short mode")
	}

	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker not found")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	scriptPath := filepath.Join(cwd, "examples", "shfmt")

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")

	// args: clix <script> --version
	args := []string{"clix", scriptPath, "--version"}

	// Use real execCommand (default)
	// We do not mock here because we want to verify the actual build and run
	// if docker is available.

	err = run(stdin, &stdout, &stderr, args)
	if err != nil {
		t.Fatalf("run failed: %v\nStderr: %s", err, stderr.String())
	}

	output := stdout.String()
	// shfmt version output usually looks like "v3.X.X"
	if !strings.Contains(output, "v3.") {
		t.Errorf("Expected version output containing 'v3.', got: %q", output)
	} else {
		t.Logf("Found shfmt version: %s", strings.TrimSpace(output))
	}
}
