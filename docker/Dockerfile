# Stage 1: Build the Go binary
FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /secondorder ./cmd/secondorder

# Stage 2: Runtime image
FROM debian:bookworm-slim

ARG USER_UID=1000
ARG USER_GID=1000
ARG NODE_MAJOR=22

# Install base dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    gnupg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js via NodeSource
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI via official apt repo
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Install AI CLI tools globally
RUN npm install -g \
    @anthropic-ai/claude-code \
    @openai/codex \
    @google/gemini-cli

# Create non-root user and group
RUN groupadd --gid ${USER_GID} so \
    && useradd --uid ${USER_UID} --gid ${USER_GID} --create-home --shell /bin/bash so

# Copy the compiled binary
COPY --from=builder /secondorder /usr/local/bin/secondorder

# Pre-create auth mount points owned by so
RUN mkdir -p \
    /home/so/.claude \
    /home/so/.codex \
    /home/so/.gemini \
    /home/so/.config/gh \
    && chown -R so:so /home/so/.claude /home/so/.codex /home/so/.gemini /home/so/.config

# Copy entrypoint script
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

USER so
WORKDIR /workspace
EXPOSE 3001

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
