package sender

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/model"
)

func NormalizeHookEvent(cfg config.Config, event model.HookEvent) (*model.DesiredEvent, error) {
	raw := strings.ToLower(strings.TrimSpace(event.RawEvent))
	if raw == "" {
		return nil, fmt.Errorf("empty raw event")
	}

	source := strings.TrimSpace(event.Source)
	if source == "" {
		source = cfg.SourceName
	}

	occurredAt := event.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	switch {
	case inSet(cfg.TrackEvents, raw):
		trackID, spotifyURI, err := normalizeTrackIdentity(event.TrackID, event.SpotifyURI)
		if err != nil {
			return nil, err
		}
		fingerprint := fingerprintFor(raw, source, occurredAt, trackID, spotifyURI)
		return &model.DesiredEvent{
			Fingerprint: fingerprint,
			Event:       "track_started",
			Source:      source,
			TrackID:     trackID,
			SpotifyURI:  spotifyURI,
			StartedAt:   ptrTime(occurredAt),
			SentAt:      occurredAt,
		}, nil
	case inSet(cfg.StopEvents, raw):
		fingerprint := fingerprintFor(raw, source, occurredAt, "", "")
		return &model.DesiredEvent{
			Fingerprint: fingerprint,
			Event:       "stopped",
			Source:      source,
			SentAt:      occurredAt,
		}, nil
	default:
		return nil, nil
	}
}

func BuildPayload(event *model.DesiredEvent) (*model.DeliveryPayload, error) {
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}
	payload := &model.DeliveryPayload{
		Event:  event.Event,
		Source: event.Source,
		SentAt: event.SentAt.UTC(),
	}

	if event.Event == "stopped" {
		return payload, nil
	}

	if event.Metadata == nil {
		return nil, fmt.Errorf("track metadata missing")
	}

	payload.TrackID = ptrString(event.TrackID)
	payload.SpotifyURI = ptrString(event.SpotifyURI)
	payload.Title = ptrString(event.Metadata.Title)
	payload.Artists = append([]string(nil), event.Metadata.Artists...)
	payload.Album = ptrString(event.Metadata.Album)
	payload.CoverURL = ptrString(event.Metadata.CoverURL)
	payload.DurationMS = ptrInt(event.Metadata.DurationMS)
	payload.StartedAt = event.StartedAt
	return payload, nil
}

func inSet(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
}

func normalizeTrackIdentity(trackID, spotifyURI string) (string, string, error) {
	trackID = strings.TrimSpace(trackID)
	spotifyURI = strings.TrimSpace(spotifyURI)

	if trackID == "" && spotifyURI == "" {
		return "", "", fmt.Errorf("track event missing track identity")
	}

	if trackID == "" && strings.HasPrefix(spotifyURI, "spotify:track:") {
		trackID = strings.TrimPrefix(spotifyURI, "spotify:track:")
	}
	if spotifyURI == "" && trackID != "" {
		spotifyURI = "spotify:track:" + trackID
	}
	if strings.HasPrefix(trackID, "spotify:track:") {
		spotifyURI = trackID
		trackID = strings.TrimPrefix(trackID, "spotify:track:")
	}
	if trackID == "" {
		return "", "", fmt.Errorf("could not derive track id")
	}
	return trackID, spotifyURI, nil
}

func fingerprintFor(rawEvent, source string, occurredAt time.Time, trackID, spotifyURI string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		rawEvent,
		source,
		occurredAt.UTC().Format(time.RFC3339Nano),
		trackID,
		spotifyURI,
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func ptrTime(value time.Time) *time.Time {
	v := value.UTC()
	return &v
}

func ptrString(value string) *string {
	v := value
	return &v
}

func ptrInt(value int) *int {
	v := value
	return &v
}
