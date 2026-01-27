package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DockerSandbox struct{}

func (s *DockerSandbox) Run(stdin io.Reader, stdout, stderr io.Writer, script Script, args []string) error {
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
		if strings.Contains(m.HostPath, "{cacheDir}") || strings.Contains(m.HostPath, "${cacheDir}") {
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
	// sha is like "sha256:"
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
		if strings.Contains(m.HostPath, "{cacheDir}") || strings.Contains(m.HostPath, "${cacheDir}") {
			if strings.Contains(m.HostPath, "{cacheDir}") {
				fmt.Fprintf(os.Stderr, "Warning: usage of {cacheDir} is deprecated and will be removed in future versions. Please use ${cacheDir} instead.\n")
			}
			if imageSHA == "" {
				return nil, fmt.Errorf("cacheDir variable used but image SHA not available")
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
			m.HostPath = strings.ReplaceAll(m.HostPath, "${cacheDir}", cacheDir)
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

		// TODO: Resolve this better once we find a container image where HOME is not /root
		if strings.HasPrefix(m.SandboxPath, "~/") {
			m.SandboxPath = "/root/" + m.SandboxPath[2:]
		} else if m.SandboxPath == "~" {
			m.SandboxPath = "/root"
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
