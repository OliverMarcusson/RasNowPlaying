package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/diag"
	"rasplayingnow/internal/model"
)

type tokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

type Client struct {
	httpClient *http.Client
	cfg        config.SpotifyConfig
	logger     *diag.Logger
	mu         sync.Mutex
	token      tokenCache
}

func NewClient(timeout time.Duration, cfg config.SpotifyConfig, logger *diag.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		cfg:        cfg,
		logger:     logger,
	}
}

func (c *Client) GetTrack(ctx context.Context, trackID string) (*model.TrackMetadata, error) {
	c.logger.Debugf("spotify track lookup start track_id=%s", trackID)
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.APIBaseURL, "/")+"/tracks/"+url.PathEscape(trackID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Errorf("spotify track lookup request failed track_id=%s err=%v", trackID, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		c.logger.Errorf("spotify track lookup failed track_id=%s status=%s body=%s", trackID, resp.Status, strings.TrimSpace(string(body)))
		return nil, fmt.Errorf("spotify track lookup returned %s: %s", resp.Status, string(body))
	}

	var track spotifyTrack
	if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
		return nil, err
	}

	artists := make([]string, 0, len(track.Artists))
	for _, artist := range track.Artists {
		if artist.Name != "" {
			artists = append(artists, artist.Name)
		}
	}

	coverURL := ""
	if len(track.Album.Images) > 0 {
		coverURL = track.Album.Images[0].URL
	}
	c.logger.Debugf("spotify track lookup success track_id=%s title=%q album=%q artists=%d", trackID, track.Name, track.Album.Name, len(artists))

	return &model.TrackMetadata{
		Title:      track.Name,
		Artists:    artists,
		Album:      track.Album.Name,
		CoverURL:   coverURL,
		DurationMS: track.DurationMS,
	}, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	if c.token.AccessToken != "" && now.Before(c.token.ExpiresAt) {
		c.logger.Debugf("spotify token cache hit expires_at=%s", c.token.ExpiresAt.Format(time.RFC3339))
		return c.token.AccessToken, nil
	}
	c.logger.Infof("requesting new spotify client-credentials token")

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(c.cfg.ClientID, c.cfg.ClientSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Errorf("spotify token request failed err=%v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		c.logger.Errorf("spotify token request failed status=%s body=%s", resp.Status, strings.TrimSpace(string(body)))
		return "", fmt.Errorf("spotify token request returned %s: %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("spotify token response missing access_token")
	}

	expiresAt := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Add(-30 * time.Second)
	c.token = tokenCache{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}
	c.logger.Infof("spotify token acquired expires_at=%s", expiresAt.Format(time.RFC3339))

	return c.token.AccessToken, nil
}

func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}

type spotifyTrack struct {
	Name       string `json:"name"`
	DurationMS int    `json:"duration_ms"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"album"`
}
