package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/sender"
	"rasplayingnow/internal/spotify"
	"rasplayingnow/internal/state"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	store, err := state.NewFileStore(cfg.StateFile)
	if err != nil {
		logger.Fatalf("create state store: %v", err)
	}

	spotifyClient := spotify.NewClient(cfg.HTTPTimeout, cfg.Spotify)
	service := sender.NewService(cfg, store, spotifyClient, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("sender stopped: %v", err)
	}
}
