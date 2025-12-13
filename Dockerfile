# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /wonder ./cmd/wonder

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Download Headscale binary
ARG HEADSCALE_VERSION=0.27.1
ARG TARGETARCH
RUN apk add --no-cache curl && \
    curl -L -o /usr/local/bin/headscale \
    "https://github.com/juanfont/headscale/releases/download/v${HEADSCALE_VERSION}/headscale_${HEADSCALE_VERSION}_linux_${TARGETARCH}" && \
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
