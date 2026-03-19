package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/diag"
	"rasplayingnow/internal/model"
	"rasplayingnow/internal/spotify"
	"rasplayingnow/internal/state"
)

type spotifyLookup interface {
	GetTrack(ctx context.Context, trackID string) (*model.TrackMetadata, error)
}

type Service struct {
	cfg        config.Config
	store      *state.FileStore
	spotify    spotifyLookup
	httpClient *http.Client
	logger     *diag.Logger
}

func NewService(cfg config.Config, store *state.FileStore, spotifyClient spotifyLookup, logger *diag.Logger) *Service {
	return &Service{
		cfg:        cfg,
		store:      store,
		spotify:    spotifyClient,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		logger:     logger,
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.logger.Infof("sender service entering run loop")
	if err := os.MkdirAll(filepath.Dir(s.cfg.SpoolFile), 0o755); err != nil {
		return fmt.Errorf("ensure spool directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.cfg.StateFile), 0o755); err != nil {
		return fmt.Errorf("ensure state directory: %w", err)
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	if err := s.tick(ctx); err != nil {
		s.logger.Errorf("initial tick failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("sender service stopping: %v", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Errorf("tick failed: %v", err)
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) error {
	s.logger.Debugf("tick start")
	stateSnapshot, err := s.store.Load()
	if err != nil {
		return err
	}
	s.logger.Debugf("state loaded pending=%t last_seen=%s last_delivered=%s attempt=%d next_attempt_at=%s",
		stateSnapshot.Pending != nil,
		shortFingerprint(stateSnapshot.LastSeenSpoolFingerprint),
		shortFingerprint(stateSnapshot.LastDeliveredFingerprint),
		stateSnapshot.Attempt,
		formatTimePtr(stateSnapshot.NextAttemptAt))

	stateSnapshot, err = s.consumeSpool(ctx, stateSnapshot)
	if err != nil {
		return err
	}

	if stateSnapshot.Pending == nil {
		s.logger.Debugf("no pending event to deliver")
		return nil
	}

	if stateSnapshot.NextAttemptAt != nil && time.Now().UTC().Before(*stateSnapshot.NextAttemptAt) {
		s.logger.Debugf("pending event waiting for retry window event=%s track_id=%s next_attempt_at=%s",
			stateSnapshot.Pending.Event, stateSnapshot.Pending.TrackID, stateSnapshot.NextAttemptAt.UTC().Format(time.RFC3339))
		return nil
	}

	return s.deliver(ctx, stateSnapshot)
}

func (s *Service) consumeSpool(ctx context.Context, stateSnapshot model.PersistedState) (model.PersistedState, error) {
	event, err := s.readHookEvent()
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Debugf("spool file does not exist yet path=%s", s.cfg.SpoolFile)
			return stateSnapshot, nil
		}
		return stateSnapshot, err
	}
	s.logger.Debugf("spool event loaded raw_event=%s source=%s occurred_at=%s track_id=%s spotify_uri=%s",
		event.RawEvent, event.Source, event.OccurredAt.UTC().Format(time.RFC3339), event.TrackID, event.SpotifyURI)

	desired, err := NormalizeHookEvent(s.cfg, event)
	if err != nil {
		s.logger.Errorf("ignore invalid hook event: %v", err)
		return stateSnapshot, nil
	}
	if desired == nil {
		s.logger.Debugf("ignoring hook event raw_event=%s because it is not in track or stop event sets", event.RawEvent)
		return stateSnapshot, nil
	}

	if desired.Fingerprint == stateSnapshot.LastSeenSpoolFingerprint || desired.Fingerprint == stateSnapshot.LastDeliveredFingerprint {
		s.logger.Debugf("ignoring duplicate event fingerprint=%s event=%s track_id=%s", shortFingerprint(desired.Fingerprint), desired.Event, desired.TrackID)
		return stateSnapshot, nil
	}

	if stateSnapshot.Pending != nil {
		s.logger.Infof("replacing pending event old_event=%s old_track_id=%s new_event=%s new_track_id=%s",
			stateSnapshot.Pending.Event, stateSnapshot.Pending.TrackID, desired.Event, desired.TrackID)
	}
	stateSnapshot.LastSeenSpoolFingerprint = desired.Fingerprint
	stateSnapshot.Pending = desired
	stateSnapshot.Attempt = 0
	now := time.Now().UTC()
	stateSnapshot.NextAttemptAt = &now

	if err := s.store.Save(stateSnapshot); err != nil {
		return stateSnapshot, err
	}

	s.logger.Infof("queued event=%s track_id=%s fingerprint=%s sent_at=%s", desired.Event, desired.TrackID, shortFingerprint(desired.Fingerprint), desired.SentAt.UTC().Format(time.RFC3339))

	if desired.Event == "track_started" {
		if err := s.ensureMetadata(ctx, &stateSnapshot); err != nil {
			s.logger.Errorf("metadata lookup deferred track_id=%s: %v", desired.TrackID, err)
		}
	}

	return stateSnapshot, nil
}

func (s *Service) deliver(ctx context.Context, stateSnapshot model.PersistedState) error {
	if stateSnapshot.Pending == nil {
		return nil
	}
	s.logger.Debugf("deliver start event=%s track_id=%s attempt=%d", stateSnapshot.Pending.Event, stateSnapshot.Pending.TrackID, stateSnapshot.Attempt+1)

	if stateSnapshot.Pending.Event == "track_started" && stateSnapshot.Pending.Metadata == nil {
		if err := s.ensureMetadata(ctx, &stateSnapshot); err != nil {
			return s.scheduleRetry(stateSnapshot, fmt.Errorf("metadata lookup: %w", err))
		}
	}

	payload, err := BuildPayload(stateSnapshot.Pending)
	if err != nil {
		return s.scheduleRetry(stateSnapshot, err)
	}

	if err := s.postPayload(ctx, payload); err != nil {
		return s.scheduleRetry(stateSnapshot, err)
	}

	delivered := stateSnapshot.Pending.Fingerprint
	stateSnapshot.Pending = nil
	stateSnapshot.Attempt = 0
	stateSnapshot.NextAttemptAt = nil
	stateSnapshot.LastDeliveredFingerprint = delivered

	if err := s.store.Save(stateSnapshot); err != nil {
		return err
	}

	s.logger.Infof("delivered event=%s track_id=%s fingerprint=%s", payload.Event, valueOrEmpty(payload.TrackID), shortFingerprint(delivered))
	return nil
}

func (s *Service) ensureMetadata(ctx context.Context, stateSnapshot *model.PersistedState) error {
	if stateSnapshot.Pending == nil || stateSnapshot.Pending.Event != "track_started" {
		return nil
	}
	if stateSnapshot.Pending.Metadata != nil {
		s.logger.Debugf("metadata already present track_id=%s", stateSnapshot.Pending.TrackID)
		return nil
	}

	s.logger.Infof("resolving spotify metadata track_id=%s", stateSnapshot.Pending.TrackID)
	metadata, err := s.spotify.GetTrack(ctx, stateSnapshot.Pending.TrackID)
	if err != nil {
		return err
	}

	stateSnapshot.Pending.Metadata = metadata
	if err := s.store.Save(*stateSnapshot); err != nil {
		return err
	}
	s.logger.Infof("spotify metadata resolved track_id=%s title=%q album=%q artists=%d duration_ms=%d cover_url_set=%t",
		stateSnapshot.Pending.TrackID, metadata.Title, metadata.Album, len(metadata.Artists), metadata.DurationMS, metadata.CoverURL != "")

	return nil
}

func (s *Service) scheduleRetry(stateSnapshot model.PersistedState, cause error) error {
	delay := s.cfg.InitialRetryDelay
	if stateSnapshot.Attempt > 0 {
		delay = s.cfg.InitialRetryDelay * time.Duration(1<<min(stateSnapshot.Attempt, 10))
	}
	if delay > s.cfg.MaxRetryDelay {
		delay = s.cfg.MaxRetryDelay
	}

	nextAttemptAt := time.Now().UTC().Add(delay)
	stateSnapshot.Attempt++
	stateSnapshot.NextAttemptAt = &nextAttemptAt

	if err := s.store.Save(stateSnapshot); err != nil {
		return err
	}

	s.logger.Errorf("retry scheduled event=%s track_id=%s attempt=%d next=%s err=%v",
		pendingEventName(stateSnapshot.Pending), pendingTrackID(stateSnapshot.Pending), stateSnapshot.Attempt, nextAttemptAt.Format(time.RFC3339), cause)
	return cause
}

func (s *Service) postPayload(ctx context.Context, payload *model.DeliveryPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.ReceiverURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.logger.Infof("posting now-playing payload receiver_url=%s event=%s track_id=%s payload=%s",
		s.cfg.ReceiverURL, payload.Event, valueOrEmpty(payload.TrackID), string(body))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Errorf("receiver request failed event=%s track_id=%s err=%v", payload.Event, valueOrEmpty(payload.TrackID), err)
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	s.logger.Infof("receiver response status=%s event=%s track_id=%s body=%s",
		resp.Status, payload.Event, valueOrEmpty(payload.TrackID), strings.TrimSpace(string(raw)))
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("receiver returned %s: %s", resp.Status, string(raw))
	}

	return nil
}

func (s *Service) readHookEvent() (model.HookEvent, error) {
	raw, err := os.ReadFile(s.cfg.SpoolFile)
	if err != nil {
		return model.HookEvent{}, err
	}
	s.logger.Debugf("read spool file path=%s bytes=%d", s.cfg.SpoolFile, len(raw))
	if len(strings.TrimSpace(string(raw))) == 0 {
		return model.HookEvent{}, fmt.Errorf("spool file is empty")
	}

	var event model.HookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return model.HookEvent{}, fmt.Errorf("decode spool event: %w", err)
	}

	return event, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func shortFingerprint(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func pendingEventName(value *model.DesiredEvent) string {
	if value == nil {
		return ""
	}
	return value.Event
}

func pendingTrackID(value *model.DesiredEvent) string {
	if value == nil {
		return ""
	}
	return value.TrackID
}

var _ spotifyLookup = (*spotify.Client)(nil)
