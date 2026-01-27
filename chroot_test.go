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
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChroot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping chroot test: not root")
	}

	tmpDir := t.TempDir()
	// Create a dummy rootfs structure.
	// We won't populate it, so execution should fail finding the binary.

	scriptPath := filepath.Join(tmpDir, "test-script-chroot")
	scriptContent := fmt.Sprintf(`#!/usr/bin/env clix
image: %s
entrypoint: /bin/echo
`, tmpDir)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")
	args := []string{"clix", scriptPath, "hello"}

	// Set env var to force chroot sandbox
	os.Setenv("CLIX_SANDBOX", "chroot")
	defer os.Unsetenv("CLIX_SANDBOX")

	err := run(stdin, &stdout, &stderr, args)

	// We expect an error.
	// 1. If we don't have CAP_SYS_CHROOT, it fails with "operation not permitted".
	// 2. If we do, it fails with "no such file or directory" because /bin/echo is not in tmpDir.
	if err == nil {
		t.Fatalf("expected error running inside empty chroot, got nil")
	}

	t.Logf("Got expected error: %v", err)
}
