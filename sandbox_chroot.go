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
	"archive/tar"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/go-containerregistry/pkg/crane"
)

type ChrootSandbox struct{}

func (s *ChrootSandbox) Run(stdin io.Reader, stdout, stderr io.Writer, script Script, args []string) error {
	rootPath := script.Image
	if rootPath == "" {
		return fmt.Errorf("ChrootSandbox requires an image path (used as root directory)")
	}

	realRoot, cleanup, err := prepareRootFS(rootPath)
	if err != nil {
		return err
	}
	defer cleanup()

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
		Chroot: realRoot,
	}

	// We start at root of the new root
	cmd.Dir = "/"

	// We are not handling environment variables here yet, or mounts.
	// Issue says: "leave a lot of functionality not supported"
	if len(script.Mounts) > 0 {
		return fmt.Errorf("mounts are not supported in chroot sandbox")
	}
	if len(script.Env) > 0 {
		return fmt.Errorf("environment variables are not supported in chroot sandbox")
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("error running chroot command: %w", err)
	}

	return nil
}

func prepareRootFS(imageRef string) (string, func(), error) {
	// Check if it is a local dir
	info, err := os.Stat(imageRef)
	if err == nil && info.IsDir() {
		return imageRef, func() {}, nil
	}

	// Assume it is a container image
	img, err := crane.Pull(imageRef)
	if err != nil {
		return "", nil, fmt.Errorf("pulling image %q: %w", imageRef, err)
	}

	tmpDir, err := os.MkdirTemp("", "clix-chroot-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	// Export to tar stream
	pr, pw := io.Pipe()
	go func() {
		err := crane.Export(img, pw)
		pw.CloseWithError(err)
	}()

	if err := untar(pr, tmpDir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("unpacking image: %w", err)
	}

	return tmpDir, cleanup, nil
}

func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, header.Name)

		// Basic zip-slip protection
		if !strings.HasPrefix(path, filepath.Clean(dest)) {
			return fmt.Errorf("illegal file path in image: %s", path)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent dir exists
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			os.Chmod(path, os.FileMode(header.Mode))
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, path); err != nil {
				return err
			}
		}
	}
	return nil
}