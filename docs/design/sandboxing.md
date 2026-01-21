# Sandboxing Design

This document describes the design for running tools in a sandbox environment (Docker) using `clix`.

## Goals

*   Allow tools to run in isolated Docker containers.
*   Support mounting host directories into the container.
*   Provide a way to dynamically determine mount points (e.g., git repository root).

## Configuration

We will add a `mounts` section to the script configuration.

```yaml
mounts:
  - hostPath: <expression>
    sandboxPath: <path> # Optional, defaults to host path
    readOnly: <boolean> # Optional, defaults to false (not implemented yet)
```

### Host Expressions

*   `git.repoRoot(cwd)`: Resolves to the root of the git repository containing the current working directory.
*   (Future) Literal paths.
*   (Future) Other dynamic expressions.

## Execution Model

When `mounts` are specified (or if sandboxing is explicitly enabled), `clix` will:

1.  Resolve the mount points.
2.  Construct a Docker command.
    *   For `go` scripts, use the `golang:latest` image.
    *   Mount the requested volumes.
    *   Set the working directory inside the container to match the current working directory (which should be inside one of the mounts).
    *   Pass the environment variables (TBD, but likely `GOCACHE`, `GOPATH` might need handling or just let them be ephemeral).
3.  Execute the command inside the container.

## Go Tools

For tools defined with `go:`, we will use the `golang` official image.

```yaml
go:
  run: github.com/example/tool
mounts:
- hostPath: git.repoRoot(cwd)
```

This will result in:

```bash
docker run \
  -v /path/to/repo:/path/to/repo \
  -w /current/work/dir \
  golang:latest \
  go run github.com/example/tool
```

```