# Project Outline

`clix` is a tool for securely running CLI tools using images, running them in containers.

## Goals

*   **Secure Execution**: Run tools in isolated environments (containers, sandboxes).
*   **Easy Installation**: managing tools via "scripts" that define how to run them.
*   **Cross-Platform**: Support for Mac, Windows, Linux.
*   **Selective Sandboxing**: Fine-grained control over what resources a tool can access (e.g., credentials).

## Prototype

The initial prototype focuses on delegating execution to `go run`. This allows for easy bootstrapping and utility for Go developers.

### Script Format

Scripts are executable files with a shebang pointing to `clix`.

Example:

```yaml
#!/usr/bin/env clix

# image: future use
go:
  run: sigs.k8s.io/kustomize/kustomize/v5
  version: v5.8.0
```

`clix` will parse this and invoke:
`go run sigs.k8s.io/kustomize/kustomize/v5@v5.8.0 <args>`
