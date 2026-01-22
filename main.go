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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

var execCommand = exec.Command

type Script struct {
	Go         *GoConfig    `json:"go,omitempty"`
	Build      *BuildConfig `json:"build,omitempty"`
	Image      string       `json:"image,omitempty"`
	Entrypoint string       `json:"entrypoint,omitempty"`
	Mounts     []Mount      `json:"mounts,omitempty"`
	Env        []EnvVar     `json:"env,omitempty"`
}

type BuildConfig struct {
	Git        string `json:"git"`
	Branch     string `json:"branch,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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

	if script.Build != nil {
		imageName, err := buildImage(stdin, stdout, stderr, script.Build)
		if err != nil {
			return fmt.Errorf("error building image: %w", err)
		}
		script.Image = imageName
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

	cmd := execCommand("docker", cmdArgs...)
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

	// Resolve cache directory if needed
	imageSHA := ""
	needsSHA := false
	for _, m := range script.Mounts {
		if strings.Contains(m.HostPath, "{cacheDir}") {
			needsSHA = true
			break
		}
	}

	if needsSHA {
		var err error
		imageSHA, err = getImageSHAFn(script.Image)
		if err != nil {
			return nil, fmt.Errorf("failed to get image SHA: %w", err)
		}
	}

	resolvedMounts, err := resolveMounts(script.Mounts, imageSHA)
	if err != nil {
		return nil, fmt.Errorf("error resolving mounts: %w", err)
	}

	for _, m := range resolvedMounts {
		cmdArgs = append(cmdArgs, "-v", fmt.Sprintf("%s:%s", m.HostPath, m.SandboxPath))
	}

	for _, e := range script.Env {
		cmdArgs = append(cmdArgs, "-e", fmt.Sprintf("%s=%s", e.Name, e.Value))
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

var getImageSHAFn = getImageSHA

func getImageSHA(image string) (string, error) {
	cmd := execCommand("docker", "images", "--no-trunc", "--quiet", image)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running docker images: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("image not found: %s", image)
	}
	// sha is like "sha256:..."
	if strings.HasPrefix(sha, "sha256:") {
		sha = sha[7:]
	}
	return sha, nil
}

func resolveMounts(mounts []Mount, imageSHA string) ([]Mount, error) {
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
		if strings.Contains(m.HostPath, "{cacheDir}") {
			if imageSHA == "" {
				return nil, fmt.Errorf("{cacheDir} used but image SHA not available")
			}
			userCache, err := os.UserCacheDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get user cache dir: %w", err)
			}
			// TODO: Eventually we'll need to do garbage collection
			cacheDir := filepath.Join(userCache, "clix", "cache", imageSHA)
			if err := os.MkdirAll(cacheDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create cache dir: %w", err)
			}
			m.HostPath = strings.ReplaceAll(m.HostPath, "{cacheDir}", cacheDir)
		}

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
	cmd := execCommand("git", "rev-parse", "--show-toplevel")
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
	cmd := execCommand("go", cmdArgs...)
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

func buildImage(stdin io.Reader, stdout, stderr io.Writer, build *BuildConfig) (string, error) {
	if build.Git == "" {
		return "", fmt.Errorf("build.git is required")
	}

	// Get the latest commit hash from the remote
	commitHash, err := getRemoteHead(build.Git, build.Branch)
	if err != nil {
		return "", fmt.Errorf("failed to get remote head: %w", err)
	}

	// Construct image tag: clix-<hash-of-repo-url>:<commit-hash>
	repoHash := sha256.Sum256([]byte(build.Git))
	repoHashStr := hex.EncodeToString(repoHash[:])[:8] // Short hash for readability

	// Extract base name for readability
	parts := strings.Split(build.Git, "/")
	baseName := parts[len(parts)-1]
	baseName = strings.TrimSuffix(baseName, ".git")
	baseName = strings.ReplaceAll(baseName, ":", "-")
	// Clean up baseName further if needed, for now assume standard repo names

	imageTag := fmt.Sprintf("clix-%s-%s:%s", baseName, repoHashStr, commitHash)

	// Check if image exists
	exists, err := imageExists(imageTag)
	if err != nil {
		return "", fmt.Errorf("failed to check if image exists: %w", err)
	}

	if exists {
		return imageTag, nil
	}

	// Clone and build
	tempDir, err := os.MkdirTemp("", "clix-build-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone
	cloneArgs := []string{"clone", "--depth", "1"}
	if build.Branch != "" {
		cloneArgs = append(cloneArgs, "--branch", build.Branch)
	}
	cloneArgs = append(cloneArgs, build.Git, tempDir)

	fmt.Fprintf(stderr, "Cloning %s...\n", build.Git)
	cmd := execCommand("git", cloneArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone failed: %w", err)
	}

	// Build
	dockerfile := "Dockerfile"
	if build.Dockerfile != "" {
		dockerfile = build.Dockerfile
	}

	buildArgs := []string{"buildx", "build", "-f", dockerfile, "--load", "--tag", imageTag, "."}

	fmt.Fprintf(stderr, "Building image %s...\n", imageTag)
	cmd = execCommand("docker", buildArgs...)
	cmd.Dir = tempDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build failed: %w", err)
	}

	return imageTag, nil
}

func getRemoteHead(repo, branch string) (string, error) {
	args := []string{"ls-remote", repo}
	if branch != "" {
		args = append(args, branch)
	} else {
		args = append(args, "HEAD")
	}

	cmd := execCommand("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no output from git ls-remote")
	}
	fields := strings.Fields(lines[0])
	if len(fields) == 0 {
		return "", fmt.Errorf("could not parse git ls-remote output")
	}
	return fields[0], nil
}

func imageExists(tag string) (bool, error) {
	cmd := execCommand("docker", "images", "-q", tag)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}
