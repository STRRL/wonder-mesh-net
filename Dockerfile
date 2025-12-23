# Build stage
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG GIT_SHA=unknown

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-s -w -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.version=${VERSION} -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.gitSHA=${GIT_SHA}" -o /wonder ./cmd/wonder

# Runtime stage
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/STRRL/wonder-mesh-net" \
      org.opencontainers.image.url="https://github.com/STRRL/wonder-mesh-net" \
      org.opencontainers.image.title="wonder-mesh-net" \
      org.opencontainers.image.description="PaaS bootstrapper turning homelab/edge machines into BYO compute"

RUN apk add --no-cache ca-certificates tzdata

# Download Headscale binary with checksum verification
ARG HEADSCALE_VERSION=0.27.1
ARG TARGETARCH
ARG HEADSCALE_SHA256_AMD64=af2a232ff407c100f05980b4d8fceaafc7fdb2e8de5eba8e184a8bb029cb6c00
ARG HEADSCALE_SHA256_ARM64=5af2bd4e18e9267b9770b94ebb60b07e6f32b586b31840b937f628d017e2722a
RUN apk add --no-cache curl && \
    curl -L -o /usr/local/bin/headscale \
    "https://github.com/juanfont/headscale/releases/download/v${HEADSCALE_VERSION}/headscale_${HEADSCALE_VERSION}_linux_${TARGETARCH}" && \
    if [ "${TARGETARCH}" = "amd64" ]; then \
        echo "${HEADSCALE_SHA256_AMD64}  /usr/local/bin/headscale" | sha256sum -c -; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        echo "${HEADSCALE_SHA256_ARM64}  /usr/local/bin/headscale" | sha256sum -c -; \
    fi && \
    chmod +x /usr/local/bin/headscale && \
    apk del curl

COPY --from=builder /wonder /usr/local/bin/wonder
COPY configs/headscale-embedded.yaml /etc/headscale/config.yaml

# Create data directories
RUN mkdir -p /data/headscale /data/coordinator

# 8080: Headscale HTTP, 9080: Coordinator HTTP
EXPOSE 8080 9080

VOLUME /data

WORKDIR /data

ENTRYPOINT ["/usr/local/bin/wonder"]
CMD ["coordinator"]
