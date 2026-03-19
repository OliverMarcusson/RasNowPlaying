package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	APIBaseURL   string
}

type Config struct {
	ReceiverURL       string
	SourceName        string
	SpoolFile         string
	StateFile         string
	PollInterval      time.Duration
	HTTPTimeout       time.Duration
	InitialRetryDelay time.Duration
	MaxRetryDelay     time.Duration
	TrackEvents       map[string]struct{}
	StopEvents        map[string]struct{}
	Spotify           SpotifyConfig
}

func FromEnv() (Config, error) {
	cfg := Config{
		ReceiverURL: os.Getenv("RECEIVER_URL"),
		SourceName:  getEnv("SOURCE_NAME", "raspotify-pi"),
		SpoolFile:   getEnv("SPOOL_FILE", "runtime/spool/current_event.json"),
		StateFile:   getEnv("STATE_FILE", "runtime/state/sender_state.json"),
		TrackEvents: splitSet(getEnv("TRACK_EVENTS", "started,playing,changed,track_changed,loading")),
		StopEvents:  splitSet(getEnv("STOP_EVENTS", "stopped,stop,session_disconnected")),
		Spotify: SpotifyConfig{
			ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
			ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
			TokenURL:     getEnv("SPOTIFY_TOKEN_URL", "https://accounts.spotify.com/api/token"),
			APIBaseURL:   getEnv("SPOTIFY_API_BASE_URL", "https://api.spotify.com/v1"),
		},
	}

	var err error
	if cfg.PollInterval, err = durationFromEnv("POLL_INTERVAL", 2*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPTimeout, err = durationFromEnv("HTTP_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.InitialRetryDelay, err = durationFromEnv("INITIAL_RETRY_DELAY", 2*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MaxRetryDelay, err = durationFromEnv("MAX_RETRY_DELAY", time.Minute); err != nil {
		return Config{}, err
	}

	switch {
	case cfg.ReceiverURL == "":
		return Config{}, fmt.Errorf("RECEIVER_URL is required")
	case cfg.Spotify.ClientID == "":
		return Config{}, fmt.Errorf("SPOTIFY_CLIENT_ID is required")
	case cfg.Spotify.ClientSecret == "":
		return Config{}, fmt.Errorf("SPOTIFY_CLIENT_SECRET is required")
	case cfg.InitialRetryDelay <= 0:
		return Config{}, fmt.Errorf("INITIAL_RETRY_DELAY must be greater than zero")
	case cfg.MaxRetryDelay < cfg.InitialRetryDelay:
		return Config{}, fmt.Errorf("MAX_RETRY_DELAY must be >= INITIAL_RETRY_DELAY")
	case cfg.PollInterval <= 0:
		return Config{}, fmt.Errorf("POLL_INTERVAL must be greater than zero")
	case cfg.HTTPTimeout <= 0:
		return Config{}, fmt.Errorf("HTTP_TIMEOUT must be greater than zero")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}

func splitSet(value string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}
