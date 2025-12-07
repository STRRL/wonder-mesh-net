FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /wonder ./cmd/wonder

FROM alpine:3.20

RUN apk add --no-cache ca-certificates sqlite

COPY --from=builder /wonder /usr/local/bin/wonder

ENTRYPOINT ["wonder"]
