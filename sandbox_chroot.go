package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

type ChrootSandbox struct{}

func (s *ChrootSandbox) Run(stdin io.Reader, stdout, stderr io.Writer, script Script, args []string) error {
	rootPath := script.Image
	if rootPath == "" {
		return fmt.Errorf("ChrootSandbox requires an image path (used as root directory)")
	}

	// Determine the command to run
	var cmdPath string
	var cmdArgs []string

	if script.Entrypoint != "" {
		cmdPath = script.Entrypoint
		cmdArgs = append([]string{cmdPath}, args...)
	} else {
		// If no entrypoint, use the first argument as command
		if len(args) > 0 {
			cmdPath = args[0]
			cmdArgs = args
		} else {
			return fmt.Errorf("no command specified and no entrypoint in script")
		}
	}

	// Prepare the command
	cmd := execCommand(cmdPath, cmdArgs[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	
	// Set SysProcAttr for chroot
	// We also need to set Credential/Setsid/etc? 
	// Ideally we should drop privileges if we are root, but that's out of scope for now.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: rootPath,
	}

	// We start at root of the new root
	cmd.Dir = "/"

	// We are not handling environment variables here yet, or mounts.
	// Issue says: "leave a lot of functionality not supported"

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("error running chroot command: %w", err)
	}

	return nil
}
