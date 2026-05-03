package letterboxd

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DiaryEntry represents a single entry from the Letterboxd diary.
type DiaryEntry struct {
	ViewingID string `json:"viewingId"` // data-viewing-id — stable diary entry ID
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Year      string `json:"year"`
	WatchedOn string `json:"watchedOn"` // "YYYY-MM-DD"
	Rating    int    `json:"rating"`    // 0-10, 0 = no rating
	Rewatch   bool   `json:"rewatch"`
}

var (
	// Each diary row: <tr class="diary-entry-row ...">
	diaryRowRe = regexp.MustCompile(`(?s)<tr[^>]+class="[^"]*diary-entry-row[^"]*"[^>]*>(.*?)</tr>`)

	// data-viewing-id="1277296195" (on the <tr> tag)
	viewingIDRe = regexp.MustCompile(`data-viewing-id="(\d+)"`)

	// data-item-slug="gone-girl"
	filmSlugRe = regexp.MustCompile(`data-item-slug="([^"]+)"`)

	// href="/films/year/2014/"
	filmYearRe = regexp.MustCompile(`href="/films/year/(\d{4})/"`)

	// href="/username/diary/films/for/2026/04/11/"
	viewingDateRe = regexp.MustCompile(`/diary/films/for/(\d{4})/(\d{2})/(\d{2})/`)

	// title in <h2 class="primaryname prettify"><a ...>Title</a>
	filmTitleRe = regexp.MustCompile(`(?s)class="primaryname[^"]*"[^>]*>[\s\S]*?<a[^>]*>([^<]+)</a>`)

	// rating from the hidden <input class="rateit-field ..." value="9">
	ratingRe = regexp.MustCompile(`class="rateit-field[^"]*"[^>]*value="(\d+)"`)

	// rewatch: the td has class icon-status-off when NOT a rewatch; absence means it IS a rewatch
	rewatchRe = regexp.MustCompile(`js-td-rewatch\b[^"]*icon-status-off`)

	// next page link: <a class="next" href="/username/diary/page/2/">
	nextPageRe = regexp.MustCompile(`class="next"`)
)

// FetchDiary scrapes the Letterboxd diary for the logged-in user, up to 60 days back.
// A 3500ms cooldown is applied between page requests.
// stopAtID is a viewing ID at which pagination stops (used by autosync); pass "" to fetch the full window.
func (c *Client) FetchDiary(username, stopAtID string) ([]DiaryEntry, error) {
	var all []DiaryEntry
	cutoff := time.Now().AddDate(0, 0, -60)
	page := 1

	for {
		url := fmt.Sprintf("https://letterboxd.com/%s/diary/page/%d/", username, page)
		log.Printf("[lb-diary] fetching page %d: %s", page, url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("building diary request: %w", err)
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Referer", "https://letterboxd.com/")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching diary page %d: %w", page, err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			break
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("diary page %d returned status %d", page, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading diary page %d: %w", page, err)
		}

		html := string(body)
		rows := diaryRowRe.FindAllStringSubmatch(html, -1)

		done := false
		for _, rowMatch := range rows {
			// Pass the full <tr>...</tr> match so parseDiaryRow can read
			// attributes on the opening tag (e.g. data-viewing-id).
			row := rowMatch[0]
			entry := parseDiaryRow(row)
			if entry == nil {
				continue
			}
			if stopAtID != "" && entry.ViewingID == stopAtID {
				done = true
				break
			}
			t, err := time.Parse("2006-01-02", entry.WatchedOn)
			if err == nil && t.Before(cutoff) {
				done = true
				break
			}
			all = append(all, *entry)
		}

		if done || !nextPageRe.MatchString(html) {
			break
		}

		page++
		time.Sleep(3500 * time.Millisecond)
	}

	log.Printf("[lb-diary] fetched %d entries across %d pages", len(all), page)
	return all, nil
}

func parseDiaryRow(row string) *DiaryEntry {
	viewingIDMatch := viewingIDRe.FindStringSubmatch(row)
	if viewingIDMatch == nil {
		return nil
	}

	entry := &DiaryEntry{ViewingID: viewingIDMatch[1]}

	if m := filmSlugRe.FindStringSubmatch(row); m != nil {
		entry.Slug = m[1]
	}
	if m := filmYearRe.FindStringSubmatch(row); m != nil {
		entry.Year = m[1]
	}
	if m := viewingDateRe.FindStringSubmatch(row); m != nil {
		entry.WatchedOn = m[1] + "-" + m[2] + "-" + m[3]
	}
	if m := filmTitleRe.FindStringSubmatch(row); m != nil {
		entry.Title = strings.TrimSpace(m[1])
	}
	if m := ratingRe.FindStringSubmatch(row); m != nil {
		fmt.Sscanf(m[1], "%d", &entry.Rating)
	}
	entry.Rewatch = !rewatchRe.MatchString(row)

	return entry
}
