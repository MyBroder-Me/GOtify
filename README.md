# GOtify

GOtify is a lightweight Go service that distributes time-limited playback URLs for HTTP Live Streaming (HLS) content. Tokens are signed with HMAC and paired with an API key check to keep media files private while remaining easy to integrate with players and automation scripts.

## Table of Contents

- [Features](#features)
- [Architecture Overview](#architecture-overview)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Clone and Install](#clone-and-install)
  - [Configuration](#configuration)
- [Running the Server](#running-the-server)
- [API Reference](#api-reference)
  - [Authentication](#authentication)
  - [`GET /token/:file`](#get-tokenfile)
  - [`GET /stream/:file/*quality`](#get-streamfilequality)
- [Security Notes](#security-notes)
- [Development](#development)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## Features

- HMAC-signed playback URLs that expire automatically.
- Mandatory API key required on every request (`X-API-Key` header).
- Built-in rate limiting (10 requests per second per client) to protect against abuse.
- Simple file server with path sanitisation to prevent directory traversal.
- Environment-based configuration for secrets and port selection.

## Architecture Overview

The service is split into several internal packages:

- `internal/security` &mdash; implements the HMAC signer used to generate and validate tokens.
- `internal/handlers` &mdash; contains HTTP handlers for issuing tokens and serving media files.
- `internal/server` &mdash; wires together middleware (logging, protection headers, rate limiting, API key enforcement) and routes.
- `cmd/server` &mdash; entry point that loads environment variables and starts the Gin HTTP server.

Media assets are read from the directory you pass to `server.New`, which defaults to `assets/audio` when using the bundled `main.go`.

## Getting Started

### Prerequisites

- Go 1.25 or newer.
- A valid API key that clients will send in the `X-API-Key` header.
- Optional: [`direnv`](https://direnv.net/) or similar if you prefer automatically loading the `.env` file.

### Clone and Install

```bash
git clone https://github.com/MyBroder-Me/GOtify GOtify
cd GOtify
go mod download
```

### Configuration

Create a `.env` file (or export the variables another way) with at least:

```ini
SECRET=your_shared_api_key
PORT=8080
```

- `SECRET` is used in two places:
  - As the API key clients must send in the `X-API-Key` header.
  - As the HMAC signing key for playback tokens.
- `PORT` defines the HTTP port (defaults to `8080` when omitted).

## Running the Server

```bash
go run ./cmd/server
```

By default the server will stream files from `assets/audio`. You can customise this by changing the argument passed to `server.New` inside `cmd/server/main.go`.

To build a binary instead:

```bash
go build -o gotify ./cmd/server
./gotify
```

## API Reference

### Authentication

Every request (including token generation and playback) must include the API key:

```
X-API-Key: <SECRET>
```

Requests that omit the header or provide an incorrect value return `401 Unauthorized`. The secret is never accepted through query parameters.

### `GET /token/:file`

Generates a signed playback URL for the requested `file`.

| Query Param | Required | Description |
|-------------|----------|-------------|
| `ttl`       | optional | Token lifetime in minutes (integer > 0). Defaults to 10 minutes. |

Sample request:

```bash
curl -H "X-API-Key: $SECRET" \
  "http://localhost:8080/token/demo?ttl=5"
```

Sample response:

```json
{
  "file": "demo",
  "expires": 1733836800,
  "url": "/stream/demo?t=6da1...&e=1733836800"
}
```

### `GET /stream/:file/*quality`

Serves the requested HLS playlist. The URL returned by `/token/:file` already contains the required query parameters:

- `t` &mdash; HMAC token.
- `e` &mdash; Unix timestamp (seconds) when the token expires.

Example playback request using the previously issued URL:

```bash
curl -H "X-API-Key: $SECRET" \
  "http://localhost:8080/stream/demo/master?t=6da1...&e=1733836800"
```

The helper `resolveFilename` automatically appends `.m3u8` when no extension is supplied, so a request for `/stream/demo` returns `master.m3u8`, whereas `/stream/demo/variant` returns `variant.m3u8`.

## Security Notes

- Tokens are validated with constant-time comparisons to mitigate timing attacks.
- Expiration timestamps are checked on both issuance and playback.
- Directory traversal is blocked (`..` segments are rejected) to ensure only files under the configured root are accessible.
- Secret negotiation via query string is disabled; use headers exclusively to avoid accidental leaks through logs or referrers.
- Rate limiting via [`tollbooth`](https://github.com/didip/tollbooth) is enabled globally. Tune the limits in `internal/server/server.go` if your deployment requires different thresholds.

## Development

- Format code: `gofmt -w <path>`
- Run tests: `go test ./...`
- Useful directories:
  - `assets/audio` &mdash; bundled sample media.
  - `internal/...` &mdash; application source code.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|--------------|-----|
| `401 Unauthorized` on every endpoint | Missing or incorrect `X-API-Key` header | Ensure clients send the same value defined in `SECRET`. |
| `500 Internal Server Error` immediately on boot | `SECRET` is empty | Set the `SECRET` environment variable. |
| `go run` fails with missing modules | Dependencies not downloaded | Run `go mod tidy` or `go mod download`. |
| Playback URL expires too quickly | `ttl` too small | Request a longer TTL when calling `/token/:file`. |

## License

This project is distributed under the terms of the [LICENSE](LICENSE) file.
