# Build stage
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG GIT_SHA=unknown

RUN apk add --no-cache gcc musl-dev nodejs npm

WORKDIR /app

# Go dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy all source files
COPY . .

# Frontend build (after COPY . . to avoid being overwritten)
RUN cd web && npm ci && npm run build
RUN rm -rf internal/app/coordinator/ui/static/assets && \
    cp -r web/dist/* internal/app/coordinator/ui/static/

# Go build
RUN CGO_ENABLED=1 go build -ldflags "-s -w -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.version=${VERSION} -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.gitSHA=${GIT_SHA}" -o /wonder ./cmd/wonder

# Runtime stage
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/STRRL/wonder-mesh-net" \
      org.opencontainers.image.url="https://github.com/STRRL/wonder-mesh-net" \
      org.opencontainers.image.title="wonder-mesh-net" \
      org.opencontainers.image.description="PaaS bootstrapper turning homelab/edge machines into BYO compute"

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /wonder /wonder

RUN mkdir -p /data/coordinator

EXPOSE 9080

VOLUME /data

WORKDIR /data

ENTRYPOINT ["/wonder"]
CMD ["coordinator"]
