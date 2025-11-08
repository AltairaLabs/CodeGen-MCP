# Python Builder Image — CodeGen-MCP (POC)

This folder contains a POC builder Docker image used by CodeGen-MCP to run Python build/test/package tasks in an isolated container.

Key features (POC):
- Pinned Python base image (via build arg).
- Non-root `builder` user to avoid running as root.
- Common Python tooling pre-installed: pip-tools, tox, pytest, mypy, flake8, build, twine.
- `PIP_CACHE_DIR` exposed so CI or local dev can mount a cache to speed repeated builds.

Build the image (example):

```bash
# from repo root
docker build -t codegen-mcp/python-builder:dev -f docker/builder/Dockerfile \
  --build-arg PYTHON_VERSION=3.11.8 \
  --build-arg DEBIAN_VERSION=bookworm-slim \
  .
```

Run the container with your workspace mounted:

```bash
docker run --rm -it \
  -v $(pwd):/home/builder/workspace \
  -v /tmp/pip-cache:/home/builder/.cache/pip \
  codegen-mcp/python-builder:dev \
  "python -m pytest -q"
```

Notes & next steps:
- For the POC we allow limited, whitelisted egress for package installs. For production, prefer a mirrored package index and stricter egress controls.
- If you want a smaller runtime image for packaging outputs only, we can implement a multi-stage Dockerfile that performs build in a full-image stage and copies artifacts into a tiny runtime stage.
- We'll add a minimal entrypoint script to validate invocations from MCP and emit structured logs for auditability.
