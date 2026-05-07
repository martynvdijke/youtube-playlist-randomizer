# YouTube Playlist Randomizer

A portable web app that shuffles (randomizes) your YouTube playlists. Built in Go as a single, self-contained binary.

Inspired by the original Python CLI version, this Go rewrite adds a web UI, real-time shuffle progress tracking, and automatic YouTube API quota management.

## Features

- **Web UI** — see all your playlists, shuffle with one click
- **Smart quota tracking** — knows your daily YouTube API limit (default 10,000 units) and shows live usage. Prevents starting shuffles that would exceed your quota.
- **Real-time progress** — watch the shuffle progress live via a modal with polling
- **Portable single binary** — compiled Go with embedded templates, no runtime dependencies
- **Docker support** — ready-to-run container image

## How it works

1. Authorize with Google (OAuth 2.0) — tokens are stored locally in SQLite
2. Pick a playlist from the list
3. Hit "Shuffle" — the app fetches all videos, randomizes the order, and creates a new private playlist named `{original}-randomized-{Month-Year}`
4. Watch the progress — each video insert is tracked against your quota

## Quick start

```bash
# 1. Get a client_secret.json from Google Cloud Console
#    (YouTube Data API v3 → Credentials → OAuth 2.0 Desktop client)

# 2. Run the binary
CLIENT_SECRET=./client_secret.json ./ypr-server

# 3. Open http://localhost:8080 and authorize with Google
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server listen port |
| `DB_PATH` | `./ypr.db` | SQLite database path |
| `CLIENT_SECRET` | `./client_secret.json` | Path to Google OAuth credentials |
| `BASE_URL` | `http://localhost:8080` | Public URL (used for OAuth redirect) |

## Docker

```bash
docker build -t ypr .
docker run -p 8080:8080 \
  -v /path/to/client_secret.json:/app/client_secret.json \
  -v ypr-data:/app/data \
  ypr
```

## API quota costs

YouTube Data API v3 has a default daily quota of **10,000 units**:

| Operation | Cost |
|---|---|
| List playlists | 1 unit/page |
| List playlist items | 1 unit/page |
| Create playlist | 50 units |
| Insert playlist item | 50 units/item |

A shuffle of a 190-video playlist costs ~9,555 units. The app checks remaining quota before starting and won't proceed if you'd exceed your daily limit.

## Development

```bash
# Build
task build

# Run with hot-reload
task dev

# Tests
task test

# Full CI pipeline
task prepush
```

Requires Go 1.26+ and `client_secret.json` from Google Cloud Console.
