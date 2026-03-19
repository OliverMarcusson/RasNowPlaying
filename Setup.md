# Setup

These are the manual steps required before `docker compose up -d --build` will work.

Important: `raspotify.service` is systemd-hardened and typically runs with `ProtectHome=true`. That means a hook script or spool path under `/home/...` can fail with `Permission denied` even if the file is executable. Use a hook path outside `/home` and write runtime files under `/var/lib/raspotify/...`.

## Runtime Paths

The sender now uses project-relative runtime paths by default:

- spool file: `runtime/spool/current_event.json`
- state file: `runtime/state/sender_state.json`

Inside Docker Compose, `${HOST_RUNTIME_DIR:-./runtime}` on the host is bind-mounted to `/app/runtime` in the container, and the Go service uses those relative paths from its working directory.

The `raspotify` hook also defaults to a script-relative spool path, but for real raspotify installs you should explicitly set `RASNOWPLAYING_SPOOL_FILE` to a writable path under `/var/lib/raspotify/...`.

## Manual Steps

1. Create a `.env` file from `.env.example`.

   Fill in at least:

   - `RECEIVER_URL`
   - `SPOTIFY_CLIENT_ID`
   - `SPOTIFY_CLIENT_SECRET`

2. Choose a host runtime directory.

   For local-only testing, you can keep the default `./runtime`.

   For actual `raspotify` integration, use:

   - `HOST_RUNTIME_DIR=/var/lib/raspotify/rasplayingnow/runtime`

   Then create the runtime directories:

```sh
sudo install -d -m 0755 /var/lib/raspotify/rasplayingnow/runtime/spool
sudo install -d -m 0755 /var/lib/raspotify/rasplayingnow/runtime/state
```

3. Create Spotify API credentials.

   You need a Spotify app that gives you a client ID and client secret for Client Credentials auth.

4. Install the hook script to a system path and configure `raspotify` to call it.

```sh
sudo install -D -m 0755 scripts/raspotify-onevent.sh /usr/local/bin/rasplayingnow-raspotify-onevent.sh
```

   Edit `/etc/raspotify/conf` and add:

```sh
LIBRESPOT_ONEVENT="/usr/local/bin/rasplayingnow-raspotify-onevent.sh"
RASNOWPLAYING_SOURCE="raspotify-pi"
RASNOWPLAYING_SPOOL_FILE="/var/lib/raspotify/rasplayingnow/runtime/spool/current_event.json"
RASNOWPLAYING_HOOK_LOG_FILE="/var/lib/raspotify/rasplayingnow/runtime/state/raspotify-onevent.log"
```

   Notes:

   - `LIBRESPOT_ONEVENT` should be an absolute path because `raspotify` is started by `systemd`.
   - Do not point `LIBRESPOT_ONEVENT` at a script under `/home/...` unless you also relax the service hardening.
   - `/var/lib/raspotify` is writable to the hardened service because the unit grants it as a `StateDirectory`.
   - Set `HOST_RUNTIME_DIR=/var/lib/raspotify/rasplayingnow/runtime` in `.env` so the sender container mounts the same files.

5. Restart `raspotify`.

```sh
sudo systemctl restart raspotify.service
```

6. Make sure file permissions are correct.

   The `raspotify` service must be able to execute `/usr/local/bin/rasplayingnow-raspotify-onevent.sh` and write to `/var/lib/raspotify/rasplayingnow/runtime/spool/current_event.json`.

7. Make sure the receiver is reachable from the sender host.

   `RECEIVER_URL` must point to a running receiver that accepts `POST /ingest/now-playing`.

8. Make sure Docker and Docker Compose are installed on the host.

9. Install the systemd unit if you want the sender stack to start on boot.

   The unit file is [systemd/rasplayingnow-sender.service](/home/oliver/src/Go/RasPlayingNow/systemd/rasplayingnow-sender.service).

   Install it with:

```sh
sudo cp /home/oliver/src/Go/RasPlayingNow/systemd/rasplayingnow-sender.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now rasplayingnow-sender.service
```

   Useful commands:

```sh
sudo systemctl status rasplayingnow-sender.service
sudo systemctl restart rasplayingnow-sender.service
sudo systemctl reload rasplayingnow-sender.service
sudo journalctl -u rasplayingnow-sender.service -f
```

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
