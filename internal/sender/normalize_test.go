package sender

import (
	"testing"
	"time"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/model"
)

func TestNormalizeTrackEvent(t *testing.T) {
	cfg := config.Config{
		SourceName:  "raspotify-pi",
		TrackEvents: map[string]struct{}{"started": {}},
		StopEvents:  map[string]struct{}{"stopped": {}},
	}

	occurredAt := time.Date(2026, 3, 19, 12, 34, 56, 0, time.UTC)
	event, err := NormalizeHookEvent(cfg, model.HookEvent{
		RawEvent:   "started",
		Source:     "",
		OccurredAt: occurredAt,
		TrackID:    "spotify:track:abc123",
	})
	if err != nil {
		t.Fatalf("NormalizeHookEvent() error = %v", err)
	}
	if event == nil {
		t.Fatal("NormalizeHookEvent() returned nil")
	}
	if event.Event != "track_started" {
		t.Fatalf("event.Event = %q", event.Event)
	}
	if event.TrackID != "abc123" {
		t.Fatalf("event.TrackID = %q", event.TrackID)
	}
	if event.SpotifyURI != "spotify:track:abc123" {
		t.Fatalf("event.SpotifyURI = %q", event.SpotifyURI)
	}
	if event.Source != "raspotify-pi" {
		t.Fatalf("event.Source = %q", event.Source)
	}
	if event.StartedAt == nil || !event.StartedAt.Equal(occurredAt) {
		t.Fatalf("event.StartedAt = %v", event.StartedAt)
	}
}

func TestNormalizeStopEvent(t *testing.T) {
	cfg := config.Config{
		SourceName:  "raspotify-pi",
		TrackEvents: map[string]struct{}{"started": {}},
		StopEvents:  map[string]struct{}{"session_disconnected": {}},
	}

	event, err := NormalizeHookEvent(cfg, model.HookEvent{
		RawEvent:   "session_disconnected",
		Source:     "kitchen-pi",
		OccurredAt: time.Date(2026, 3, 19, 12, 40, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NormalizeHookEvent() error = %v", err)
	}
	if event == nil {
		t.Fatal("NormalizeHookEvent() returned nil")
	}
	if event.Event != "stopped" {
		t.Fatalf("event.Event = %q", event.Event)
	}
	if event.TrackID != "" {
		t.Fatalf("event.TrackID = %q", event.TrackID)
	}
}
