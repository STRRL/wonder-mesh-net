# Build stage
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG GIT_SHA=unknown

RUN apk add --no-cache gcc musl-dev nodejs npm

WORKDIR /app

# Frontend build first (better layer caching)
COPY web/package*.json web/
RUN cd web && npm ci

COPY web/ web/
RUN cd web && npm run build
RUN mkdir -p internal/app/coordinator/ui/static && \
    cp -r web/dist/* internal/app/coordinator/ui/static/

# Go build
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

COPY --from=builder /wonder /wonder

RUN mkdir -p /data/coordinator

EXPOSE 9080

VOLUME /data

WORKDIR /data

ENTRYPOINT ["/wonder"]
CMD ["coordinator"]
