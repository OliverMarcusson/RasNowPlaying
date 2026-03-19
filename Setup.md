# Setup

These are the manual steps required before `docker compose up -d --build` will work.

## Runtime Paths

The sender now uses project-relative runtime paths by default:

- spool file: `runtime/spool/current_event.json`
- state file: `runtime/state/sender_state.json`

Inside Docker Compose, `./runtime` on the host is bind-mounted to `/app/runtime` in the container, and the Go service uses those relative paths from its working directory.

The `raspotify` hook also defaults to the repo-relative spool path when you point `LIBRESPOT_ONEVENT` at the script in this repository.

## Manual Steps

1. Create a `.env` file from `.env.example`.

   Fill in at least:

   - `RECEIVER_URL`
   - `SPOTIFY_CLIENT_ID`
   - `SPOTIFY_CLIENT_SECRET`

2. Create the runtime directories in the repo root.

```sh
mkdir -p runtime/spool runtime/state
```

3. Create Spotify API credentials.

   You need a Spotify app that gives you a client ID and client secret for Client Credentials auth.

4. Configure `raspotify` to call the hook script from this repo.

   Edit `/etc/raspotify/conf` and add:

```sh
LIBRESPOT_ONEVENT="/absolute/path/to/RasPlayingNow/scripts/raspotify-onevent.sh"
RASNOWPLAYING_SOURCE="raspotify-pi"
```

   Notes:

   - `LIBRESPOT_ONEVENT` should be an absolute path because `raspotify` is started by `systemd`.
   - You do not need to set `RASNOWPLAYING_SPOOL_FILE` if the script stays in this repo, because it will write to `runtime/spool/current_event.json` relative to the repository layout.
   - If you move the script somewhere else, then also set `RASNOWPLAYING_SPOOL_FILE` to the absolute path of this repo's spool file.

5. Restart `raspotify`.

```sh
sudo systemctl restart raspotify.service
```

6. Make sure file permissions are correct.

   The user running `raspotify` must be able to execute `scripts/raspotify-onevent.sh` and write to `runtime/spool/current_event.json`.

7. Make sure the receiver is reachable from the sender host.

   `RECEIVER_URL` must point to a running receiver that accepts `POST /ingest/now-playing`.

8. Make sure Docker and Docker Compose are installed on the host.

## Quick Verification

Before starting the stack, verify:

- `.env` exists and has valid credentials
- `runtime/spool` exists
- `runtime/state` exists
- `/etc/raspotify/conf` points to this repo's hook script
- `sudo systemctl restart raspotify.service` succeeds
- the receiver URL is reachable from the host

After that, start the sender with:

```sh
docker compose up -d --build
```
