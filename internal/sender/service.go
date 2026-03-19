package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"rasplayingnow/internal/config"
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
	logger     *log.Logger
}

func NewService(cfg config.Config, store *state.FileStore, spotifyClient spotifyLookup, logger *log.Logger) *Service {
	return &Service{
		cfg:        cfg,
		store:      store,
		spotify:    spotifyClient,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		logger:     logger,
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.SpoolFile), 0o755); err != nil {
		return fmt.Errorf("ensure spool directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.cfg.StateFile), 0o755); err != nil {
		return fmt.Errorf("ensure state directory: %w", err)
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	if err := s.tick(ctx); err != nil {
		s.logger.Printf("initial tick failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Printf("tick failed: %v", err)
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) error {
	stateSnapshot, err := s.store.Load()
	if err != nil {
		return err
	}

	stateSnapshot, err = s.consumeSpool(ctx, stateSnapshot)
	if err != nil {
		return err
	}

	if stateSnapshot.Pending == nil {
		return nil
	}

	if stateSnapshot.NextAttemptAt != nil && time.Now().UTC().Before(*stateSnapshot.NextAttemptAt) {
		return nil
	}

	return s.deliver(ctx, stateSnapshot)
}

func (s *Service) consumeSpool(ctx context.Context, stateSnapshot model.PersistedState) (model.PersistedState, error) {
	event, err := s.readHookEvent()
	if err != nil {
		if os.IsNotExist(err) {
			return stateSnapshot, nil
		}
		return stateSnapshot, err
	}

	desired, err := NormalizeHookEvent(s.cfg, event)
	if err != nil {
		s.logger.Printf("ignore invalid hook event: %v", err)
		return stateSnapshot, nil
	}
	if desired == nil {
		return stateSnapshot, nil
	}

	if desired.Fingerprint == stateSnapshot.LastSeenSpoolFingerprint || desired.Fingerprint == stateSnapshot.LastDeliveredFingerprint {
		return stateSnapshot, nil
	}

	stateSnapshot.LastSeenSpoolFingerprint = desired.Fingerprint
	stateSnapshot.Pending = desired
	stateSnapshot.Attempt = 0
	now := time.Now().UTC()
	stateSnapshot.NextAttemptAt = &now

	if err := s.store.Save(stateSnapshot); err != nil {
		return stateSnapshot, err
	}

	s.logger.Printf("queued event=%s track_id=%s", desired.Event, desired.TrackID)

	if desired.Event == "track_started" {
		if err := s.ensureMetadata(ctx, &stateSnapshot); err != nil {
			s.logger.Printf("metadata lookup deferred track_id=%s: %v", desired.TrackID, err)
		}
	}

	return stateSnapshot, nil
}

func (s *Service) deliver(ctx context.Context, stateSnapshot model.PersistedState) error {
	if stateSnapshot.Pending == nil {
		return nil
	}

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

	s.logger.Printf("delivered event=%s track_id=%s", payload.Event, valueOrEmpty(payload.TrackID))
	return nil
}

func (s *Service) ensureMetadata(ctx context.Context, stateSnapshot *model.PersistedState) error {
	if stateSnapshot.Pending == nil || stateSnapshot.Pending.Event != "track_started" {
		return nil
	}
	if stateSnapshot.Pending.Metadata != nil {
		return nil
	}

	metadata, err := s.spotify.GetTrack(ctx, stateSnapshot.Pending.TrackID)
	if err != nil {
		return err
	}

	stateSnapshot.Pending.Metadata = metadata
	if err := s.store.Save(*stateSnapshot); err != nil {
		return err
	}

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

	s.logger.Printf("retry scheduled attempt=%d next=%s err=%v", stateSnapshot.Attempt, nextAttemptAt.Format(time.RFC3339), cause)
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

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("receiver returned %s: %s", resp.Status, string(raw))
	}

	return nil
}

func (s *Service) readHookEvent() (model.HookEvent, error) {
	raw, err := os.ReadFile(s.cfg.SpoolFile)
	if err != nil {
		return model.HookEvent{}, err
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

var _ spotifyLookup = (*spotify.Client)(nil)
