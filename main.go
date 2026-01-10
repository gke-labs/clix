package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"sigs.k8s.io/yaml"
)

type Script struct {
	Go *GoConfig `json:"go"`
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

	if script.Go == nil {
		return fmt.Errorf("error: 'go' configuration missing in script")
	}

	goPackage := script.Go.Run
	version := script.Go.Version

	if goPackage == "" {
		return fmt.Errorf("error: 'go.run' missing in script")
	}

	target := goPackage
	if version != "" {
		target = fmt.Sprintf("%s@%s", goPackage, version)
	}

	cmdArgs := append([]string{"run", target}, scriptArgs...)
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
