# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN set -eux; \
    out=/out/opt/loopforge; mkdir -p "$out"; \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o "$out/loopforge" ./cmd/loopforge; \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o "$out/loopforge-mcp" ./cmd/loopforge-mcp; \
    for os in linux darwin; do for arch in amd64 arm64; do \
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out/loopforge-runner-$os-$arch" ./cmd/loopforge-runner; \
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out/loopforge-gate-mcp-$os-$arch" ./cmd/loopforge-gate-mcp; \
    done; done

FROM docker:28-cli AS docker-cli

FROM node:22-bookworm-slim
# Agent CLIs executed by the built-in executors. Set to "false" for a slim
# control-plane image that only dispatches to remote runners.
ARG INSTALL_AGENT_CLIS=true
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates git openssh-client bubblewrap curl tini; \
    rm -rf /var/lib/apt/lists/*; \
    if [ "$INSTALL_AGENT_CLIS" = "true" ]; then \
      npm install -g @anthropic-ai/claude-code @openai/codex @tencent-ai/codebuddy-code; \
      npm cache clean --force; \
    fi; \
    useradd --create-home --uid 10001 loopforge; \
    mkdir -p /data/home && chown -R loopforge:loopforge /data

# Docker CLI for per-run sandbox containers when the host's Docker socket is
# mounted (see docker-compose.yml); without the socket it stays unused.
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker-cli /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx
COPY --from=build /out/ /

# HOME lives on the data volume so CLI logins survive upgrades and can be
# shared with sandbox containers.
ENV LOOPFORGE_DATA_DIR=/data \
    HOME=/data/home \
    LOOPFORGE_HTTP_PORT=8080 \
    LOOPFORGE_RUNNER_BINARY=/opt/loopforge/loopforge-runner-{os}-{arch} \
    LOOPFORGE_GATE_MCP_BINARY=/opt/loopforge/loopforge-gate-mcp-{os}-{arch} \
    PATH=/opt/loopforge:$PATH

USER loopforge
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--", "loopforge"]
