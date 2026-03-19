package state

import (
	"path/filepath"
	"testing"
	"time"

	"rasplayingnow/internal/model"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "sender_state.json"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	now := time.Date(2026, 3, 19, 12, 34, 56, 0, time.UTC)
	state := model.PersistedState{
		Pending: &model.DesiredEvent{
			Fingerprint: "abc",
			Event:       "track_started",
			Source:      "raspotify-pi",
			TrackID:     "track123",
			SpotifyURI:  "spotify:track:track123",
			SentAt:      now,
			StartedAt:   &now,
			Metadata: &model.TrackMetadata{
				Title:      "Song Title",
				Artists:    []string{"Artist 1"},
				Album:      "Album",
				CoverURL:   "https://i.scdn.co/image/example",
				DurationMS: 210000,
			},
		},
		LastSeenSpoolFingerprint: "abc",
		LastDeliveredFingerprint: "xyz",
		Attempt:                  2,
		NextAttemptAt:            &now,
	}

	if err := store.Save(state); err != nil {
		t.Fatalf("store.Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load() error = %v", err)
	}
	if loaded.Pending == nil || loaded.Pending.TrackID != "track123" {
		t.Fatalf("loaded.Pending = %#v", loaded.Pending)
	}
	if loaded.LastSeenSpoolFingerprint != "abc" {
		t.Fatalf("loaded.LastSeenSpoolFingerprint = %q", loaded.LastSeenSpoolFingerprint)
	}
}
