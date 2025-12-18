package namethattune

import (
	"fmt"
	"net/url"
	"strings"
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
