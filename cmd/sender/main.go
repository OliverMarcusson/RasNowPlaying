package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/diag"
	"rasplayingnow/internal/sender"
	"rasplayingnow/internal/spotify"
	"rasplayingnow/internal/state"
)

func main() {
	baseLogger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	if err := config.LoadDotEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		baseLogger.Fatalf("FATAL load .env: %v", err)
	}

	cfg, err := config.FromEnv()
	if err != nil {
		baseLogger.Fatalf("FATAL load config: %v", err)
	}
	logger := diag.New(baseLogger, cfg.LogLevel)
	logger.Infof("starting sender log_level=%s receiver_url=%s source_name=%s spool_file=%s state_file=%s poll_interval=%s http_timeout=%s retry_initial=%s retry_max=%s spotify_api_base=%s spotify_token_url=%s",
		cfg.LogLevel, cfg.ReceiverURL, cfg.SourceName, cfg.SpoolFile, cfg.StateFile, cfg.PollInterval, cfg.HTTPTimeout, cfg.InitialRetryDelay, cfg.MaxRetryDelay, cfg.Spotify.APIBaseURL, cfg.Spotify.TokenURL)
	logger.Debugf("spotify credentials configured client_id_set=%t client_secret_set=%t", cfg.Spotify.ClientID != "", cfg.Spotify.ClientSecret != "")

	store, err := state.NewFileStore(cfg.StateFile)
	if err != nil {
		logger.Fatalf("create state store: %v", err)
	}

	spotifyClient := spotify.NewClient(cfg.HTTPTimeout, cfg.Spotify, logger)
	service := sender.NewService(cfg, store, spotifyClient, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("sender stopped: %v", err)
	}
	logger.Infof("sender shut down")
}
