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
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChroot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping chroot test: not root")
	}

	tmpDir := t.TempDir()

	scriptPath := filepath.Join(tmpDir, "test-script-chroot-pull")

	// hello-world binary is at /hello
	scriptContent := `#!/usr/bin/env clix
image: hello-world
entrypoint: /hello
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")
	args := []string{"clix", scriptPath}

	os.Setenv("CLIX_SANDBOX", "chroot")
	defer os.Unsetenv("CLIX_SANDBOX")

	err := run(stdin, &stdout, &stderr, args)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "Hello from Docker!") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}
