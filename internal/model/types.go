package model

import "time"

type HookEvent struct {
	RawEvent   string    `json:"raw_event"`
	Source     string    `json:"source"`
	OccurredAt time.Time `json:"occurred_at"`
	TrackID    string    `json:"track_id,omitempty"`
	SpotifyURI string    `json:"spotify_uri,omitempty"`
}

type TrackMetadata struct {
	Title      string   `json:"title"`
	Artists    []string `json:"artists"`
	Album      string   `json:"album"`
	CoverURL   string   `json:"cover_url"`
	DurationMS int      `json:"duration_ms"`
}

type DesiredEvent struct {
	Fingerprint string         `json:"fingerprint"`
	Event       string         `json:"event"`
	Source      string         `json:"source"`
	TrackID     string         `json:"track_id,omitempty"`
	SpotifyURI  string         `json:"spotify_uri,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	SentAt      time.Time      `json:"sent_at"`
	Metadata    *TrackMetadata `json:"metadata,omitempty"`
}

type DeliveryPayload struct {
	Event      string     `json:"event"`
	Source     string     `json:"source"`
	SentAt     time.Time  `json:"sent_at"`
	TrackID    *string    `json:"track_id,omitempty"`
	SpotifyURI *string    `json:"spotify_uri,omitempty"`
	Title      *string    `json:"title,omitempty"`
	Artists    []string   `json:"artists,omitempty"`
	Album      *string    `json:"album,omitempty"`
	CoverURL   *string    `json:"cover_url,omitempty"`
	DurationMS *int       `json:"duration_ms,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
}

type PersistedState struct {
	Pending                  *DesiredEvent `json:"pending,omitempty"`
	LastSeenSpoolFingerprint string        `json:"last_seen_spool_fingerprint,omitempty"`
	LastDeliveredFingerprint string        `json:"last_delivered_fingerprint,omitempty"`
	Attempt                  int           `json:"attempt,omitempty"`
	NextAttemptAt            *time.Time    `json:"next_attempt_at,omitempty"`
}
