package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"rasplayingnow/internal/config"
	"rasplayingnow/internal/diag"
)

func TestGetTrack(t *testing.T) {
	logger := diag.New(logDiscarder(t), "debug")
	client := NewClient(5*time.Second, config.SpotifyConfig{
		ClientID:     "client",
		ClientSecret: "secret",
		TokenURL:     "https://spotify.test/token",
		APIBaseURL:   "https://spotify.test/v1",
	}, logger)
	client.httpClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("client:secret"))
				if got := r.Header.Get("Authorization"); got != expected {
					t.Fatalf("token auth header = %q", got)
				}
				return jsonResponse(http.StatusOK, map[string]any{
					"access_token": "token123",
					"expires_in":   3600,
				}), nil
			case "/v1/tracks/track123":
				if got := r.Header.Get("Authorization"); got != "Bearer token123" {
					t.Fatalf("track auth header = %q", got)
				}
				return jsonResponse(http.StatusOK, map[string]any{
					"name":        "Song Title",
					"duration_ms": 210000,
					"artists": []map[string]any{
						{"name": "Artist 1"},
						{"name": "Artist 2"},
					},
					"album": map[string]any{
						"name": "Album Name",
						"images": []map[string]any{
							{"url": "https://i.scdn.co/image/example"},
						},
					},
				}), nil
			default:
				t.Fatalf("unexpected request path %q", r.URL.Path)
				return nil, nil
			}
		}),
	}

	track, err := client.GetTrack(context.Background(), "track123")
	if err != nil {
		t.Fatalf("GetTrack() error = %v", err)
	}
	if track.Title != "Song Title" {
		t.Fatalf("track.Title = %q", track.Title)
	}
	if track.Album != "Album Name" {
		t.Fatalf("track.Album = %q", track.Album)
	}
	if len(track.Artists) != 2 {
		t.Fatalf("len(track.Artists) = %d", len(track.Artists))
	}
	if track.CoverURL != "https://i.scdn.co/image/example" {
		t.Fatalf("track.CoverURL = %q", track.CoverURL)
	}
	if track.DurationMS != 210000 {
		t.Fatalf("track.DurationMS = %d", track.DurationMS)
	}
}

func logDiscarder(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, payload any) *http.Response {
	raw, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
