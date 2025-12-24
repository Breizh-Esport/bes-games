package namethattune

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ExtractYouTubeID parses a YouTube URL and extracts the video ID.
//
// Supported forms:
// - https://www.youtube.com/watch?v=<id>
// - https://youtube.com/watch?v=<id>
// - https://m.youtube.com/watch?v=<id>
// - https://youtu.be/<id>
// - https://www.youtube.com/embed/<id>
// - https://www.youtube.com/shorts/<id>
//
// It does not validate that the video exists; it only validates that the ID
// matches a safe pattern.
func ExtractYouTubeID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty youtube url")
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid youtube url")
	}

	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	// youtu.be/<id>
	if host == "youtu.be" {
		id := strings.Trim(strings.TrimPrefix(u.Path, "/"), " ")
		id = strings.Trim(id, "/")
		if isYouTubeID(id) {
			return id, nil
		}
		return "", fmt.Errorf("invalid youtube id")
	}

	// youtube.com/*
	if strings.HasSuffix(host, "youtube.com") {
		// /watch?v=<id>
		if strings.HasPrefix(u.Path, "/watch") {
			id := strings.TrimSpace(u.Query().Get("v"))
			if isYouTubeID(id) {
				return id, nil
			}
			return "", fmt.Errorf("invalid youtube id")
		}

		// /embed/<id>
		if strings.HasPrefix(u.Path, "/embed/") {
			id := strings.TrimPrefix(u.Path, "/embed/")
			id = strings.Trim(id, "/")
			if isYouTubeID(id) {
				return id, nil
			}
			return "", fmt.Errorf("invalid youtube id")
		}

		// /shorts/<id>
		if strings.HasPrefix(u.Path, "/shorts/") {
			id := strings.TrimPrefix(u.Path, "/shorts/")
			id = strings.Trim(id, "/")
			if isYouTubeID(id) {
				return id, nil
			}
			return "", fmt.Errorf("invalid youtube id")
		}
	}

	return "", fmt.Errorf("unsupported youtube url")
}

type YouTubeMetadata struct {
	Title        string
	ThumbnailURL string
}

// FetchYouTubeMetadata calls YouTube's oEmbed endpoint to get basic metadata.
func FetchYouTubeMetadata(ctx context.Context, rawURL string) (YouTubeMetadata, error) {
	if os.Getenv("BES_YOUTUBE_OEMBED_DISABLE") == "1" {
		return YouTubeMetadata{Title: "Unknown title", ThumbnailURL: ""}, nil
	}

	if _, err := ExtractYouTubeID(rawURL); err != nil {
		return YouTubeMetadata{}, err
	}

	endpoint := "https://www.youtube.com/oembed"
	q := url.Values{}
	q.Set("url", rawURL)
	q.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return YouTubeMetadata{}, fmt.Errorf("build metadata request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return YouTubeMetadata{}, fmt.Errorf("fetch metadata: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return YouTubeMetadata{}, fmt.Errorf("metadata lookup failed with status %d", res.StatusCode)
	}

	var payload struct {
		Title        string `json:"title"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return YouTubeMetadata{}, fmt.Errorf("decode metadata: %w", err)
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		return YouTubeMetadata{}, fmt.Errorf("metadata title missing")
	}

	return YouTubeMetadata{
		Title:        title,
		ThumbnailURL: strings.TrimSpace(payload.ThumbnailURL),
	}, nil
}

// isYouTubeID performs a conservative validation of a YouTube video ID.
// Typical IDs are 11 chars, but we accept a wider range while staying url-safe.
func isYouTubeID(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) < 6 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
}
