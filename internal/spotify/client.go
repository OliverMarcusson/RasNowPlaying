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
	"rasplayingnow/internal/model"
)

type tokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

type Client struct {
	httpClient *http.Client
	cfg        config.SpotifyConfig
	mu         sync.Mutex
	token      tokenCache
}

func NewClient(timeout time.Duration, cfg config.SpotifyConfig) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		cfg:        cfg,
	}
}

func (c *Client) GetTrack(ctx context.Context, trackID string) (*model.TrackMetadata, error) {
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
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
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
		return c.token.AccessToken, nil
	}

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
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
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
