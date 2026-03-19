#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PROJECT_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
DEFAULT_SPOOL_FILE="$PROJECT_ROOT/runtime/spool/current_event.json"

SPOOL_FILE="${RASNOWPLAYING_SPOOL_FILE:-$DEFAULT_SPOOL_FILE}"
SOURCE_NAME="${RASNOWPLAYING_SOURCE:-raspotify-pi}"
RAW_EVENT="${PLAYER_EVENT:-unknown}"
TRACK_ID_VALUE="${TRACK_ID:-}"
SPOTIFY_URI_VALUE="${SPOTIFY_URI:-}"

if [ -z "$SPOTIFY_URI_VALUE" ] && [ -n "$TRACK_ID_VALUE" ]; then
  case "$TRACK_ID_VALUE" in
    spotify:track:*)
      SPOTIFY_URI_VALUE="$TRACK_ID_VALUE"
      TRACK_ID_VALUE="${TRACK_ID_VALUE#spotify:track:}"
      ;;
    *)
      SPOTIFY_URI_VALUE="spotify:track:$TRACK_ID_VALUE"
      ;;
  esac
fi

OCCURRED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
SPOOL_DIR="$(dirname "$SPOOL_FILE")"
mkdir -p "$SPOOL_DIR"

TMP_FILE="$(mktemp "$SPOOL_DIR/.current_event.XXXXXX")"
cleanup() {
  rm -f "$TMP_FILE"
}
trap cleanup EXIT INT TERM

json_escape() {
  printf '%s' "$1" | sed \
    -e 's/\\/\\\\/g' \
    -e 's/"/\\"/g'
}

{
  printf '{\n'
  printf '  "raw_event": "%s",\n' "$(json_escape "$RAW_EVENT")"
  printf '  "source": "%s",\n' "$(json_escape "$SOURCE_NAME")"
  printf '  "occurred_at": "%s"' "$(json_escape "$OCCURRED_AT")"
  if [ -n "$TRACK_ID_VALUE" ]; then
    printf ',\n  "track_id": "%s"' "$(json_escape "$TRACK_ID_VALUE")"
  fi
  if [ -n "$SPOTIFY_URI_VALUE" ]; then
    printf ',\n  "spotify_uri": "%s"' "$(json_escape "$SPOTIFY_URI_VALUE")"
  fi
  printf '\n}\n'
} > "$TMP_FILE"

mv "$TMP_FILE" "$SPOOL_FILE"
