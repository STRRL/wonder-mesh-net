# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

wonder-mesh-net is a Go-based mesh networking project that provides L3 virtual networking with NAT traversal using pure socket APIs (no TUN device permissions required).

**Primary Technology Choice**: Tailscale/Headscale + tsnet for Go-native socket API with WireGuard-based L3 overlay network and DERP relay fallback.

**Alternative**: ZeroTier + libzt when multi-language bindings or smaller binary size is needed.

## Build Commands

```bash
# Build the project
go build ./...

# Run tests
go test ./...

# Run a single test
go test -run TestName ./path/to/package

# Run tests with verbose output
go test -v ./...

# Check for issues
go vet ./...
```

## Code Style

- No end-of-line comments
- No Chinese in code comments
