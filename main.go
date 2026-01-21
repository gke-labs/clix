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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

type Script struct {
	Go         *GoConfig     `json:"go,omitempty"`
	Python     *PythonConfig `json:"python,omitempty"`
	Image      string        `json:"image,omitempty"`
	Entrypoint string        `json:"entrypoint,omitempty"`
	Mounts     []Mount       `json:"mounts,omitempty"`
}

type PythonConfig struct {
	Cache bool `json:"cache,omitempty"`
}

type Mount struct {
	HostPath    string `json:"hostPath"`
	SandboxPath string `json:"sandboxPath,omitempty"`
}

type GoConfig struct {
	Run     string `json:"run"`
	Version string `json:"version,omitempty"`
}

func main() {
	if err := run(os.Stdin, os.Stdout, os.Stderr, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: %s <script> [args...]", args[0])
	}

	scriptPath := args[1]
	scriptArgs := args[2:]

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("error reading script file: %w", err)
	}

	var script Script
	if err := yaml.Unmarshal(data, &script); err != nil {
		return fmt.Errorf("error parsing script file: %w", err)
	}

	if script.Image != "" {
		return runDocker(stdin, stdout, stderr, script, scriptArgs)
	}

	if script.Go != nil {
		if len(script.Mounts) > 0 {
			// Transform into a Docker script
			script.Image = "golang:latest"
			// We need to construct the command arguments for `go run ...`
			goPackage := script.Go.Run
			if script.Go.Version != "" {
				goPackage = fmt.Sprintf("%s@%s", goPackage, script.Go.Version)
			}
			// Prepend "go", "run", goPackage to the user arguments
			// Note: We don't set Entrypoint because runDocker appends Image then Args.
			// So `docker run ... golang:latest go run pkg args...` works.
			newArgs := append([]string{"go", "run", goPackage}, scriptArgs...)
			return runDocker(stdin, stdout, stderr, script, newArgs)
		}
		return runGo(stdin, stdout, stderr, script.Go, scriptArgs)
	}

	return fmt.Errorf("error: script configuration missing (expected 'go' or 'image')")
}

func runDocker(stdin io.Reader, stdout, stderr io.Writer, script Script, args []string) error {
	cmdArgs, err := buildDockerArgs(script, args, isTerminal(stdin))
	if err != nil {
		return fmt.Errorf("error building docker args: %w", err)
	}

	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Propagate the exit code from the subcommand
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("error running docker command: %w", err)
	}
	return nil
}

func buildDockerArgs(script Script, args []string, isTerm bool) ([]string, error) {
	cmdArgs := []string{"run", "-i"}
	if isTerm {
		cmdArgs = append(cmdArgs, "-t")
	}

	if script.Python != nil && script.Python.Cache {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user cache dir: %w", err)
		}
		hostCache := filepath.Join(userCache, "clix", "python")
		if err := os.MkdirAll(hostCache, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache dir: %w", err)
		}
		containerCache := "/tmp/.clix-pycache"

		script.Mounts = append(script.Mounts, Mount{
			HostPath:    hostCache,
			SandboxPath: containerCache,
		})

		cmdArgs = append(cmdArgs, "-e", fmt.Sprintf("PYTHONPYCACHEPREFIX=%s", containerCache))
	}

	resolvedMounts, err := resolveMounts(script.Mounts)
	if err != nil {
		return nil, fmt.Errorf("error resolving mounts: %w", err)
	}

	for _, m := range resolvedMounts {
		cmdArgs = append(cmdArgs, "-v", fmt.Sprintf("%s:%s", m.HostPath, m.SandboxPath))
	}

	// Set working directory to CWD if possible
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current working directory: %w", err)
	}
	cmdArgs = append(cmdArgs, "-w", cwd)

	if script.Entrypoint != "" {
		cmdArgs = append(cmdArgs, "--entrypoint", script.Entrypoint)
	}
	cmdArgs = append(cmdArgs, script.Image)
	cmdArgs = append(cmdArgs, args...)

	return cmdArgs, nil
}

func resolveMounts(mounts []Mount) ([]Mount, error) {
	var resolved []Mount
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home dir: %w", err)
	}

	for _, m := range mounts {
		if m.HostPath == "git.repoRoot(cwd)" {
			root, err := findGitRoot(cwd)
			if err != nil {
				return nil, fmt.Errorf("failed to find git root: %w", err)
			}
			m.HostPath = root
		}

		if strings.HasPrefix(m.HostPath, "~/") {
			m.HostPath = filepath.Join(home, m.HostPath[2:])
		} else if m.HostPath == "~" {
			m.HostPath = home
		}

		if m.SandboxPath == "" {
			m.SandboxPath = m.HostPath
		}
		resolved = append(resolved, m)
	}
	return resolved, nil
}

func findGitRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func runGo(stdin io.Reader, stdout, stderr io.Writer, config *GoConfig, args []string) error {
	goPackage := config.Run
	version := config.Version

	if goPackage == "" {
		return fmt.Errorf("error: 'go.run' missing in script")
	}

	target := goPackage
	if version != "" {
		target = fmt.Sprintf("%s@%s", goPackage, version)
	}

	cmdArgs := append([]string{"run", target}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Propagate the exit code from the subcommand
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("error running command: %w", err)
	}

	return nil
}
