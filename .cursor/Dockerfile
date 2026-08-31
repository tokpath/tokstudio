# TokenHub Cloud Agent base image.
# Stable system dependencies live here; repo-specific work happens in scripts/cloud-install.sh.
# Docker is intentionally not used at runtime: the backend runs natively against local
# PostgreSQL and Redis, matching the "没有 Docker 时" native flow documented in README.md.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git build-essential gnupg sudo tzdata \
        postgresql postgresql-contrib redis-server \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22 (matches web/ toolchain: Next.js 15 / React 19).
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Go 1.27 (required by backend/go.mod: `go 1.27.0`; Ubuntu apt only ships older Go).
RUN curl -fsSL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz -o /tmp/go.tgz \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz

# Ensure the runtime user exists and can manage the local PostgreSQL/Redis services.
RUN if ! id ubuntu >/dev/null 2>&1; then useradd -m -s /bin/bash ubuntu; fi \
    && echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-ubuntu \
    && chmod 0440 /etc/sudoers.d/90-ubuntu \
    && mkdir -p /go && chown ubuntu:ubuntu /go

ENV GOPATH=/go
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"

USER ubuntu
