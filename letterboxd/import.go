package letterboxd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ImportFilm represents a single film entry for the match-import request.
type ImportFilm struct {
	Title         string  `json:"title"`
	OriginalTitle string  `json:"originalTitle"`
	Rating        float64 `json:"rating"`
	Review        *string `json:"review"`
	Year          int     `json:"year"`
	IMDbID        *string `json:"imdbId"`
	LetterboxdURI *string `json:"letterboxdURI"`
	TMDbID        *string `json:"tmdbId"`
	Tags          *string `json:"tags"`
	WatchedDate   string  `json:"watchedDate"`
	IsICMImport   bool    `json:"isICheckMoviesImport"`
	Rewatch       bool    `json:"rewatch"`
	Creators      []any   `json:"creators"`
}

// MatchedFilm is the result of matching: the Letterboxd film ID for a given import entry.
type MatchedFilm struct {
	Index        int
	FilmID       string
	WatchedDate  string
	Rating       float64
	ShouldImport bool
}

// ImportResult contains the outcome of the save step.
type ImportResult struct {
	Success int
	Total   int
	Message string
}

// MatchFilms sends films to Letterboxd's match-import endpoint and parses the returned film IDs from HTML.
func (c *Client) MatchFilms(films []ImportFilm) ([]MatchedFilm, error) {
	// Build the JSON payload matching what the browser sends
	payload := map[string]any{
		"importType":  "diary",
		"importFilms": films,
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling import payload: %w", err)
	}

	data := url.Values{}
	data.Set("json", string(jsonBytes))
	data.Set("__csrf", c.CSRFToken)

	req, err := http.NewRequest("POST", "https://letterboxd.com/import/watchlist/match-import-film/", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building match request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", "https://letterboxd.com")
	req.Header.Set("Referer", "https://letterboxd.com/import/csv/")
	req.Header.Set("Accept", "text/html, */*; q=0.01")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("match request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[lb-import] match response status: %d, body length: %d", resp.StatusCode, len(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("match returned status %d", resp.StatusCode)
	}

	html := string(body)

	// Update CSRF if present in response cookies
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "com.xk72.webparts.csrf" {
			c.CSRFToken = cookie.Value
		}
	}

	return parseMatchedFilms(html, films), nil
}

// parseMatchedFilms extracts film IDs from the match response HTML.
func parseMatchedFilms(html string, films []ImportFilm) []MatchedFilm {
	// Split HTML into per-film blocks by <div class="matched-item">
	blocks := strings.Split(html, `<div class="matched-item">`)

	filmIDRe := regexp.MustCompile(`<input type="hidden" name="importFilmId" value="(\d+)"`)
	duplicateRe := regexp.MustCompile(`data-duplicate="true"`)

	var matched []MatchedFilm
	idx := 0
	for _, block := range blocks {
		m := filmIDRe.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		isDuplicate := duplicateRe.MatchString(block)

		watchedDate := ""
		var rating float64
		if idx < len(films) {
			watchedDate = films[idx].WatchedDate
			rating = films[idx].Rating
		}
		matched = append(matched, MatchedFilm{
			Index:        idx,
			FilmID:       m[1],
			WatchedDate:  watchedDate,
			Rating:       rating,
			ShouldImport: !isDuplicate,
		})
		idx++
	}

	log.Printf("[lb-import] matched %d/%d films", len(matched), len(films))
	return matched
}

// SaveImport sends the matched films to Letterboxd's save endpoint.
func (c *Client) SaveImport(matched []MatchedFilm) (*ImportResult, error) {
	data := url.Values{}
	data.Set("__csrf", c.CSRFToken)
	data.Set("filmListId", "")
	data.Set("name", "")
	data.Set("publicList", "")
	data.Set("numberedList", "")
	data.Set("notes", "")
	data.Set("tags", "")
	data.Set("shouldMarkAsWatched", "true")
	data.Set("shouldImportWatchedDates", "true")
	data.Set("shouldImportRatings", "true")

	for _, m := range matched {
		data.Add("importFilmId", m.FilmID)
		data.Add("importViewingId", "")
		data.Add("shouldImportFilm", fmt.Sprintf("%t", m.ShouldImport))
		data.Add("importWatchedDate", m.WatchedDate)
		if m.Rating > 0 {
			data.Add("importRating", fmt.Sprintf("%.1f", m.Rating))
		} else {
			data.Add("importRating", "")
		}
		data.Add("importReview", "")
		data.Add("importTags", "")
		data.Add("importRewatch", "false")
	}

	req, err := http.NewRequest("POST", "https://letterboxd.com/s/save-users-imported-imdb-history", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building save request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", "https://letterboxd.com")
	req.Header.Set("Referer", "https://letterboxd.com/import/csv/")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("save request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[lb-import] save response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("save returned status %d: %s", resp.StatusCode, string(body))
	}

	return &ImportResult{
		Success: len(matched),
		Total:   len(matched),
		Message: string(body),
	}, nil
}
