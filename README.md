# RasPlayingNow Sender

This repository now contains the sender side of the service described in `NowPlaying.md`.

The sender is split into two parts:

- a host-side `raspotify` hook script that exits quickly and writes the latest event to a spool file
- a standalone Go daemon that reads that spool file, resolves Spotify track metadata, and POSTs the now-playing payload to the receiver

The daemon is designed to run under Docker Compose while the `raspotify` hook remains outside the container. The integration point is a bind-mounted host directory shared by both.

## Layout

- `cmd/sender`: Go entrypoint
- `internal/...`: sender implementation
- `scripts/raspotify-onevent.sh`: host-side hook script for `LIBRESPOT_ONEVENT`
- `docker-compose.yml`: sender compose setup
- `.env.example`: required runtime environment variables
- `runtime/spool` and `runtime/state`: default shared bind-mounted directories

## Quick Start

1. Copy `.env.example` to `.env` and fill in:
   - `RECEIVER_URL`
   - `SPOTIFY_CLIENT_ID`
   - `SPOTIFY_CLIENT_SECRET`
   - optionally set `LOG_LEVEL=debug` while diagnosing sender issues
2. Create the shared directories if they do not already exist:
   - `runtime/spool`
   - `runtime/state`
3. Point `raspotify` at the host hook script.
4. Start the sender with `docker compose up -d --build`.

## Raspotify Hook Setup

Point `raspotify` at the repo script directly, or symlink to it from a stable host path, then add this to `/etc/raspotify/conf`:

```sh
LIBRESPOT_ONEVENT="/absolute/path/to/RasPlayingNow/scripts/raspotify-onevent.sh"
RASNOWPLAYING_SOURCE="raspotify-pi"
```

By default the hook writes to `../runtime/spool/current_event.json` relative to the script location, so if you keep the script in this repo you do not need to set `RASNOWPLAYING_SPOOL_FILE`.

For host-side hook diagnostics, you can also set:

```sh
RASNOWPLAYING_HOOK_LOG_FILE="/absolute/path/to/RasPlayingNow/runtime/state/raspotify-onevent.log"
```

That file records each hook invocation and the key librespot variables the script received.

Then restart raspotify:

```sh
sudo systemctl restart raspotify.service
```

The container reads the same file through the bind mount configured in `docker-compose.yml`.

## Docker Compose

The default compose file mounts `./runtime` into `/app/runtime` inside the container and reads configuration from shell environment or `.env`:

- host spool file: `./runtime/spool/current_event.json`
- host state file: `./runtime/state/sender_state.json`
- container spool file: `runtime/spool/current_event.json`
- container state file: `runtime/state/sender_state.json`

Start it with:

```sh
docker compose up -d --build
```

View logs with:

```sh
docker compose logs -f sender
```

For deeper diagnostics, set `LOG_LEVEL=debug` in `.env` and restart the stack. The sender will then log spool polling, event normalization, dedupe decisions, Spotify lookups, POST attempts, retries, and receiver responses.

## systemd

If you want the compose stack to start at boot, install [systemd/rasplayingnow-sender.service](/home/oliver/src/Go/RasPlayingNow/systemd/rasplayingnow-sender.service):

```sh
sudo cp /home/oliver/src/Go/RasPlayingNow/systemd/rasplayingnow-sender.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now rasplayingnow-sender.service
```

## Behavior

- latest-only queue: only one pending event is kept
- stop events replace any unsent track-start event
- pending state survives container restarts
- duplicate hook events are ignored using a stable event fingerprint
- track metadata is resolved with Spotify Client Credentials

## Test

```sh
go test ./...
```
