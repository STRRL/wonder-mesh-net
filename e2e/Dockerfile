FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /wonder ./cmd/wonder

FROM alpine:3.20

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    iptables \
    ip6tables \
    iproute2 \
    curl

ARG HEADSCALE_VERSION=0.27.1
ARG TAILSCALE_VERSION=1.92.1
ARG TARGETARCH=arm64
RUN curl -L -o /usr/local/bin/headscale \
    "https://github.com/juanfont/headscale/releases/download/v${HEADSCALE_VERSION}/headscale_${HEADSCALE_VERSION}_linux_${TARGETARCH}" && \
    chmod +x /usr/local/bin/headscale && \
    if [ "$TARGETARCH" = "arm64" ]; then TS_ARCH="arm64"; else TS_ARCH="amd64"; fi && \
    curl -fsSL "https://pkgs.tailscale.com/stable/tailscale_${TAILSCALE_VERSION}_${TS_ARCH}.tgz" -o /tmp/tailscale.tgz && \
    tar xzf /tmp/tailscale.tgz -C /tmp && \
    find /tmp -name tailscale -type f -exec cp {} /usr/local/bin/ \; && \
    find /tmp -name tailscaled -type f -exec cp {} /usr/local/bin/ \; && \
    rm -rf /tmp/tailscale*

COPY --from=builder /wonder /usr/local/bin/wonder
COPY e2e/headscale-config.yaml /etc/headscale/config.yaml

RUN mkdir -p /data/headscale /data/coordinator

WORKDIR /data

ENTRYPOINT ["wonder"]
