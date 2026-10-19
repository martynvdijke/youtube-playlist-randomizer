# youtube-playlist-randomizer

> Takes a YouTube playlist, shuffles it, and saves it as a new playlist

[![Go Version](https://img.shields.io/github/go-mod/go-version/martynvdijke/youtube-playlist-randomizer)](https://github.com/martynvdijke/youtube-playlist-randomizer)

Since the Chromecast cannot shuffle a YouTube playlist, this tool provides a web UI to randomize your playlists via the YouTube Data API v3.

## Features

- **Web UI** — HTMX-powered interface to browse, filter, and randomize playlists
- **Quota management** — Tracks YouTube API quota usage and auto-pauses/resumes jobs when quota resets
- **Job persistence** — SQLite-backed job state survives server restarts
- **Docker support** — Ready-to-run container image
- **Mock mode** — Run without YouTube API credentials for development

## Quick Start

### Prerequisites

1. A [Google Cloud project](https://developers.google.com/youtube/v3/quickstart/go) with the YouTube Data API v3 enabled
2. An OAuth 2.0 client ID for a desktop application (download as `client_secret.json`)

### Using Docker

```sh
docker run -p 6270:6270 \
  -e OAUTH_CALLBACK_URL=http://localhost:6270/callback \
  -v /path/to/client_secret.json:/config/client_secret.json \
  -v ypr-data:/db \
  ghcr.io/martynvdijke/ypr:latest
```

### From Source

```sh
go build -o ypr-server .
./ypr-server -i client_secret.json
```

Open http://localhost:6270 and authorize with your Google account.

## CLI Options

| Flag | Default | Description |
|------|---------|-------------|
| `-p` | `6270` | Port to listen on |
| `-i` | `client_secret.json` | Path to OAuth client secret JSON |
| `-d` | `.` | Data directory for DB and cached token |
| `-mock` | `false` | Run in mock mode (no YouTube API) |
| `-version` | | Print version and exit |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER` | `false` | Toggles Docker-specific paths and OAuth port behaviour |
| `PORT` | `6270` | OAuth callback port (used when `DOCKER=true`) |
| `OAUTH_CALLBACK_URL` | — | Custom OAuth redirect URL. Set to an HTTP address (e.g. `http://localhost:6270/callback`) when the callback server must be reachable from outside the container. The listener binds to `0.0.0.0` so the port must be mapped. |
| `ADMIN_PASSWORD` | — | Enables the login page for admin and mutating routes. **Set this whenever the app is reachable from another machine.** When unset, those routes only accept loopback clients and requests carrying proxy forwarding headers are denied. |
| `SESSION_SECRET` | random per start | Key used to sign admin session cookies. Set a long random value to keep sessions valid across restarts. |
| `COOKIE_SECURE` | `false` | Set to `true` when served over HTTPS so session and CSRF cookies get the `Secure` flag. Also enabled automatically when `OAUTH_CALLBACK_URL` starts with `https://`. |
| `CORS_ALLOWED_ORIGIN` | — | Single origin allowed to call the API cross-origin. When unset, browsers get no CORS headers (same-origin only). |
| `SETTINGS_ENCRYPTION_KEY` | — | Encrypts integration secrets (Gotify token) at rest with AES-256-GCM. Without it secrets are stored as plaintext and are still masked in API/HTML responses. Existing plaintext values keep working and are encrypted the next time they are saved. |
| `OTEL_SERVICE_NAME` | `youtube-playlist-randomizer` | OpenTelemetry service name |
| `VERSION` | — | Override version reported in telemetry |

### Security

- Admin and state-changing endpoints require a signed `HttpOnly` session cookie obtained from `/login`.
- OAuth `state` values are random and single-use.
- The Gotify token is never returned by the API or settings form; leave the token field blank to keep the existing value.
- The OAuth token file is written with `0600` permissions.

## Development

```sh
# Build and run
go build -o ypr-server .
./ypr-server -mock

# With hot reload (requires air)
air -c .air.toml

# Build TypeScript
npx tsc

# Run tests
go test -v ./...
npx playwright test

# Lint
go vet ./...
```

## API

The server exposes REST endpoints under `/api/`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/quota` | GET | Current API quota usage |
| `/api/playlists` | GET | List user's playlists |
| `/api/randomize` | POST | Start a randomize job |
| `/api/jobs/{id}` | GET | Job status |

See [static/swagger.json](static/swagger.json) for the full API specification.

## How it works

1. Lists your YouTube playlists via the Data API
2. Fetches all video IDs from the selected playlist
3. Shuffles them using Fisher-Yates algorithm
4. Creates a new playlist with the shuffled order
5. Inserts items with automatic pause/resume when API quota is exhausted

## [Changelog](CHANGELOG.md)

## License

[MIT](https://choosealicense.com/licenses/mit/)
