# Stage 1: Build frontend
FROM node:22-alpine AS frontend

WORKDIR /app/webui

COPY webui/package*.json ./
RUN npm ci

COPY webui/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG GIT_SHA=unknown

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY --from=frontend /app/webui/dist/ ./internal/app/coordinator/webui/static/

RUN CGO_ENABLED=1 go build -ldflags "-s -w -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.version=${VERSION} -X github.com/strrl/wonder-mesh-net/cmd/wonder/commands.gitSHA=${GIT_SHA}" -o /wonder ./cmd/wonder

# Stage 3: Runtime
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
