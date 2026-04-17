package plex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const discoverBase = "https://discover.provider.plex.tv"

// DiscoverClient talks to the Plex Discover provider API
// (discover.provider.plex.tv) for universal metadata search,
// scrobbling, and rating.
type DiscoverClient struct {
	Token    string // X-Plex-Token
	ClientID string // X-Plex-Client-Identifier
}

// SearchResult represents a single movie match from the Discover search API.
type SearchResult struct {
	RatingKey string `json:"ratingKey"` // metadata ID, e.g. "64dd290c84713e6f8ba2874b"
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"` // "movie"
	GUID      string `json:"guid"`
}

// SearchMovie searches the Plex Discover API for a movie by title+year.
// Returns matching movie results ordered by relevance score.
func (c *DiscoverClient) SearchMovie(title string, year int) ([]SearchResult, error) {
	query := title
	if year > 0 {
		query = fmt.Sprintf("%s %d", title, year)
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", "10")
	params.Set("searchTypes", "movies")
	params.Set("searchProviders", "discover")
	params.Set("includeMetadata", "1")

	reqURL := fmt.Sprintf("%s/library/search?%s", discoverBase, params.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover search: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover search: HTTP %d: %s", resp.StatusCode, body)
	}

	// Response: MediaContainer.SearchResults[].SearchResult[].Metadata
	var container struct {
		MediaContainer struct {
			SearchResults []struct {
				SearchResult []struct {
					Metadata SearchResult `json:"Metadata"`
					Score    float64      `json:"score"`
				} `json:"SearchResult"`
			} `json:"SearchResults"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("discover search: parse: %w", err)
	}

	var results []SearchResult
	for _, group := range container.MediaContainer.SearchResults {
		for _, r := range group.SearchResult {
			if r.Metadata.RatingKey != "" && r.Metadata.Type == "movie" {
				results = append(results, r.Metadata)
			}
		}
	}
	return results, nil
}

// Scrobble marks a movie as watched on the Plex Discover provider.
// metadataID is the Discover metadata ID (e.g. "64dd290c84713e6f8ba2874b").
func (c *DiscoverClient) Scrobble(metadataID string) error {
	params := url.Values{}
	params.Set("identifier", "tv.plex.provider.discover")
	params.Set("key", metadataID)

	reqURL := fmt.Sprintf("%s/actions/scrobble?%s", discoverBase, params.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("scrobble: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scrobble: HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// Rate sets a rating (1-10) for a movie on the Plex Discover provider.
// metadataID is the Discover metadata ID.
func (c *DiscoverClient) Rate(metadataID string, rating int) error {
	params := url.Values{}
	params.Set("identifier", "tv.plex.provider.discover")
	params.Set("key", metadataID)
	params.Set("rating", fmt.Sprintf("%d", rating))

	reqURL := fmt.Sprintf("%s/actions/rate?%s", discoverBase, params.Encode())
	req, err := http.NewRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rate: HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (c *DiscoverClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Set("X-Plex-Token", c.Token)
	q.Set("X-Plex-Client-Identifier", c.ClientID)
	q.Set("X-Plex-Product", "Plexterboxd")
	q.Set("X-Plex-Language", "en")
	req.URL.RawQuery = q.Encode()
}

var httpClient = &http.Client{Timeout: 15 * time.Second}
