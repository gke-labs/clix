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

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestE2E(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("Skipping E2E test; set RUN_E2E=1 to run")
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	tmpDir, err := os.MkdirTemp("", "clix-e2e-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	clixPath := filepath.Join(tmpDir, "clix")

	// Build clix
	buildCmd := exec.Command("go", "build", "-o", clixPath, repoRoot)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build clix: %v\nOutput: %s", err, out)
	}

	// Set up PATH
	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	// Run example
	shfmtExample := filepath.Join(repoRoot, "examples", "shfmt")
	runCmd := exec.Command(shfmtExample, "--version")
	if out, err := runCmd.CombinedOutput(); err != nil {
		// In some environments, this might fail due to Docker rate limits.
		// We log the error but it might cause the test to fail.
		t.Errorf("shfmt --version failed: %v\nOutput: %s", err, out)
	} else {
		t.Logf("shfmt --version succeeded: %s", out)
	}
}
