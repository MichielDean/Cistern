# ─── Stage 1: build ──────────────────────────────────────────────────────────
FROM golang:1.26 AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /src

# Download dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Build both binaries. CGO_ENABLED=1 is required by go-sqlite3.
# The output binaries are dynamically linked against glibc — debian:bookworm-slim
# is the correct runtime pairing.
COPY . .
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}" -o /out/ct ./cmd/ct \
    && go build -o /out/aqueduct ./cmd/aqueduct

# ─── Stage 2: runtime ────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

# Install runtime dependencies.
RUN apt-get update && apt-get install -y --no-install-recommends \
        tmux \
        git \
        nodejs \
        npm \
        openssh-client \
        curl \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install gh CLI from the official GitHub APT repository.
# $(dpkg --print-architecture) resolves to amd64 or arm64 automatically.
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Redirect gh auth state into the named volume so it survives container restarts.
# GH_CONFIG_DIR is an officially supported gh env var.
ENV GH_CONFIG_DIR=/root/.cistern/auth/gh

# Binaries from the builder stage.
COPY --from=builder /out/ct /usr/local/bin/ct
COPY --from=builder /out/aqueduct /usr/local/bin/aqueduct

# Entrypoint script.
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

VOLUME ["/root/.cistern"]

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["ct", "castellarius", "start"]
