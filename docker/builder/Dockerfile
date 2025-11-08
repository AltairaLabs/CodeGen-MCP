# Multi-stage Python builder image for CodeGen-MCP (POC)
# - Purpose: provide a reproducible, minimal environment for building, testing
#   and packaging Python projects driven by MCP tool calls.
# - Notes: POC-focused: pinned base, non-root user, pip cache mount support,
#   common build/test tools installed.

ARG PYTHON_VERSION=3.11.8
ARG DEBIAN_VERSION=bookworm-slim
FROM python:${PYTHON_VERSION}-${DEBIAN_VERSION} as base

# Metadata for reproducible builds
ARG BUILD_DATE=""
ARG VCS_REF=""
LABEL org.opencontainers.image.created=$BUILD_DATE \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.title="codegen-mcp-python-builder" \
      org.opencontainers.image.description="Builder image for CodeGen-MCP POC"

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=off \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_DEFAULT_TIMEOUT=60

# Install OS-level build deps and useful tools
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential \
       git \
       curl \
       ca-certificates \
       libssl-dev \
       libffi-dev \
       python3-venv \
       jq \
       gnupg \
       wget \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for builds
ARG BUILDER_UID=1000
ARG BUILDER_GID=1000
RUN groupadd -g ${BUILDER_GID} builder \
    && useradd -m -u ${BUILDER_UID} -g builder -s /bin/bash builder

WORKDIR /home/builder/workspace

# Install Python tooling (pip, pip-tools, tox, pytest, mypy, flake8)
RUN python -m pip install --upgrade pip setuptools wheel
RUN pip install \
    pip-tools==7.13.0 \
    tox==4.8.0 \
    pytest==7.4.0 \
    pytest-cov==4.0.0 \
    mypy==1.9.0 \
    flake8==6.0.0 \
    build==0.11.0 \
    twine==4.0.1

# Create a directory for pip cache that can be mounted from the host
ENV PIP_CACHE_DIR=/home/builder/.cache/pip
RUN mkdir -p ${PIP_CACHE_DIR} \
    && chown -R builder:builder /home/builder

# Switch to non-root
USER builder

# Expose a simple entrypoint script that runs commands inside the workspace
# Use bash to allow command chaining; MCP will call the container with explicit
# commands / arguments.
ENTRYPOINT ["/bin/bash", "-lc"]
CMD ["echo 'Builder container ready. Run commands with: docker run -v $(pwd):/home/builder/workspace ... <image> \"<your-command>\"' && sleep infinity"]
