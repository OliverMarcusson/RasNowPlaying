# Push-Based Now Playing API With Receiver Hooks

## Summary
Build this as two standalone services:

- **Sender Pi (raspotify Pi):** `raspotify` emits local track-change events; a sender daemon enriches them with Spotify metadata and POSTs a complete now-playing payload to the receiver.
- **Receiver Pi:** a Python HTTP service accepts the POST, stores the latest now-playing state, and exposes both:
  - a read API for clients to fetch current state
  - a **hook mechanism** so each accepted POST can immediately update your local webapp state

Locked decisions in this plan:

- Metadata is resolved on the **sender Pi**
- The receiver is a **Python service**
- The receiver is **LAN-only with no auth**
- Album art is represented as a **Spotify image URL**
- The receiver stores **latest state only**

Important behavioral choice: the sender must send both track-start and stop/clear events so the receiver and the webapp do not serve stale now-playing data after playback ends.

## Interfaces And Data Flow
### Sender-side flow
- Configure `raspotify` to call a lightweight hook script via `LIBRESPOT_ONEVENT`.
- The hook script must exit quickly and only hand off normalized event data locally.
- A sender daemon reads those local events, deduplicates them, resolves Spotify metadata, and POSTs to the receiver.
- The sender daemon uses Spotify Web API `GET /v1/tracks/{id}` with **Client Credentials** auth to resolve:
  - `title`
  - `artists[]`
  - `album`
  - `cover_url`
  - `duration_ms`

### Receiver-side flow
- The receiver accepts incoming now-playing POSTs.
- It validates and normalizes the payload.
- It atomically updates its latest-state store.
- It then runs one or more **post-ingest hooks** inside the receiver process.
- Those hooks are the integration point for the local webapp and are triggered only after the new state has been accepted and stored.

### Hook model on receiver
Implement receiver hooks as an internal plugin/callback layer, not as shell hooks.

- Define a receiver-side hook interface such as `on_now_playing_update(normalized_payload, stored_state)`.
- The HTTP ingest handler calls all registered hooks after persistence succeeds.
- Hook failures must **not** roll back the accepted POST or corrupt stored state.
- Hook execution result should be logged, with per-hook success/failure.
- The webapp integration should be one hook implementation; additional hooks can be added later without changing the HTTP API.

Recommended first hook strategy for the webapp:

- If the webapp runs in the same backend process space or can be imported, call its state update function directly.
- If the webapp has its own backend on the same Pi, call a local internal function/module boundary first; only use a localhost HTTP callback if direct integration is not possible.
- Avoid browser-only mechanisms as the primary integration path.

### Receiver API
Implement these endpoints on the receiver Pi:

- `POST /ingest/now-playing`
  - Called only by the sender Pi
  - Accepts JSON payload
  - Validates required fields
  - Stores latest state
  - Triggers registered post-ingest hooks
  - Returns `204 No Content` if state persistence succeeded, even if a non-critical hook fails
- `GET /api/now-playing`
  - Returns the latest known now-playing object
- `GET /healthz`
  - Returns `200 OK` when the receiver service is healthy
- `GET /readyz`
  - Returns ready only when the state store is writable and hook registry initialized

### JSON contract
For `POST /ingest/now-playing`, use this payload shape:

```json
{
  "event": "track_started",
  "source": "raspotify-pi",
  "sent_at": "2026-03-19T12:34:56Z",
  "track_id": "spotify_track_id",
  "spotify_uri": "spotify:track:...",
  "title": "Song Title",
  "artists": ["Artist 1", "Artist 2"],
  "album": "Album Name",
  "cover_url": "https://i.scdn.co/image/...",
  "duration_ms": 210000,
  "started_at": "2026-03-19T12:34:52Z"
}
```

Stop event:

```json
{
  "event": "stopped",
  "source": "raspotify-pi",
  "sent_at": "2026-03-19T12:40:00Z"
}
```

For `GET /api/now-playing`, always return a stable shape:

```json
{
  "is_playing": true,
  "event": "track_started",
  "source": "raspotify-pi",
  "track_id": "spotify_track_id",
  "spotify_uri": "spotify:track:...",
  "title": "Song Title",
  "artists": ["Artist 1", "Artist 2"],
  "album": "Album Name",
  "cover_url": "https://i.scdn.co/image/...",
  "duration_ms": 210000,
  "started_at": "2026-03-19T12:34:52Z",
  "sent_at": "2026-03-19T12:34:56Z",
  "updated_at": "2026-03-19T12:34:56Z"
}
```

Stopped/empty response:

```json
{
  "is_playing": false,
  "event": "stopped",
  "source": "raspotify-pi",
  "track_id": null,
  "spotify_uri": null,
  "title": null,
  "artists": [],
  "album": null,
  "cover_url": null,
  "duration_ms": null,
  "started_at": null,
  "sent_at": "2026-03-19T12:40:00Z",
  "updated_at": "2026-03-19T12:40:00Z"
}
```

## Implementation Changes
### Sender Pi
- Add a `raspotify` hook script that captures minimal event data from `librespot` environment variables and writes it to a local spool location.
- Add a sender daemon, managed by `systemd`, responsible for:
  - reading the spool/event file
  - resolving Spotify metadata
  - constructing the POST payload
  - retrying delivery when the receiver is unavailable
- Use a latest-only retry strategy:
  - if multiple unsent track events accumulate, keep only the newest one
  - stop events replace any pending track payload
- Persist minimal sender state locally so a reboot does not lose the last unsent payload.
- Configure a static receiver URL such as `http://receiver-pi.local:8787/ingest/now-playing`.

### Receiver Pi
- Implement a Python HTTP service, managed by `systemd`.
- Store latest state in memory and persist it atomically to a single JSON file.
- Bind on `0.0.0.0` for LAN access.
- Add a receiver hook registry:
  - hooks are Python callables registered at service startup
  - hooks receive the normalized now-playing payload and the persisted current-state object
  - hooks run synchronously in v1 so the webapp sees the update immediately after ingest
- The webapp update path should be implemented as a dedicated hook adapter module, not embedded in the HTTP handler.
- Log hook execution time and failures.
- If hook latency becomes a problem later, the design can move to an internal queue, but v1 remains synchronous for deterministic state propagation.

### Failure and consistency rules
- The receiver must treat state persistence as the source of truth.
- If persistence fails, return `500` and do not run hooks.
- If persistence succeeds and a hook fails:
  - keep the stored state
  - return `204`
  - log the hook failure for diagnosis
- Hooks must be idempotent with respect to repeated payloads because sender retries may replay the latest payload.

## Test Plan
### Functional scenarios
- New track starts on raspotify:
  - sender enriches and POSTs metadata
  - receiver stores latest state
  - receiver runs the webapp hook
  - `GET /api/now-playing` returns the new track
- Playback stops:
  - sender sends `stopped`
  - receiver clears latest state
  - receiver hook updates the webapp to the stopped/empty state
- Same track event repeats:
  - sender deduplicates and does not flood the receiver
  - if a retry occurs anyway, receiver hook remains safe due to idempotency
- Receiver offline during a track change:
  - sender retains latest unsent payload and retries
- Receiver restart:
  - last known state is restored from disk
  - hooks re-register on startup

### Hook-specific tests
- Registered hook receives normalized payload after successful persistence
- Hook failure does not prevent `GET /api/now-playing` from reflecting the new state
- Multiple hooks run in deterministic order
- Replayed payload does not break webapp state updates
- Long-running hook behavior is logged and visible in service logs

### Acceptance criteria
- Another LAN client can fetch `GET /api/now-playing` and receive artist, song title, album cover URL, and song length for the current track.
- The receiver updates within a few seconds of a track change on raspotify.
- The local webapp on the receiver Pi is updated on each accepted incoming POST.
- The receiver does not serve stale data after playback stops.
- Temporary receiver outages do not permanently lose the latest state.

## Assumptions And Defaults
- No existing repo or service layout was provided, so this is planned as two standalone services with an integration hook on the receiver.
- The sender Pi has network access to Spotify Web API for metadata lookup.
- Public track metadata is sufficient; no user-scoped playback API is required.
- v1 does not track pause/resume/seek position continuously.
- “Hookable” on the receiver means **application-level callbacks inside the receiver service**, with the webapp update implemented through that callback layer.
