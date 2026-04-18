package plex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const accountHistoryEndpoint = "https://community.plex.tv/api"

const watchHistoryQuery = `
    query GetWatchHistoryHub($uuid: ID = "", $first: PaginationInt!, $after: String, $skipUserState: Boolean = false) {
  user(id: $uuid) {
    watchHistory(first: $first, after: $after) {
      nodes {
        metadataItem {
          ...itemFields
        }
        date
        id
      }
      pageInfo {
        hasNextPage
        hasPreviousPage
        endCursor
      }
    }
  }
}
    
    fragment itemFields on MetadataItem {
  id
  images {
    coverArt
    coverPoster
    thumbnail
    art
  }
  userState @skip(if: $skipUserState) {
    viewCount
    viewedLeafCount
    watchlistedAt
  }
  title
  key
  type
  index
  publicPagesURL
  parent {
    ...parentFields
  }
  grandparent {
    ...parentFields
  }
  publishedAt
  leafCount
  year
  originallyAvailableAt
  childCount
}
    

    fragment parentFields on MetadataItem {
  index
  title
  publishedAt
  key
  type
  images {
    coverArt
    coverPoster
    thumbnail
    art
  }
  userState @skip(if: $skipUserState) {
    viewCount
    viewedLeafCount
    watchlistedAt
  }
}
    `

// AccountHistoryEntry represents a single entry from the account-wide watch history.
type AccountHistoryEntry struct {
	ID           string `json:"id"`     // watch history entry UUID (community.plex.tv)
	Date         string `json:"date"`   // ISO 8601, e.g. "2024-07-13T20:11:00.000Z"
	Rating       int    `json:"rating"` // 1-10, 0 means no rating
	MetadataItem struct {
		ID                    string `json:"id"`
		Key                   string `json:"key"`                   // "/library/metadata/27977"
		Title                 string `json:"title"`
		Type                  string `json:"type"`                  // "MOVIE" or "EPISODE"
		Year                  int    `json:"year"`
		OriginallyAvailableAt string `json:"originallyAvailableAt"` // "YYYY-MM-DD"
		PublicPagesURL        string `json:"publicPagesURL"`
	} `json:"metadataItem"`
}

// RatingKey returns the Discover metadata ID for this entry.
// This is the hex ID (e.g. "64dd290c84713e6f8ba2874b") used by
// the Discover provider API for scrobble, rate, and search.
func (e *AccountHistoryEntry) RatingKey() string {
	return e.MetadataItem.ID
}

type graphqlRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
	OperationName string         `json:"operationName"`
}

type graphqlResponse struct {
	Data struct {
		User struct {
			WatchHistory struct {
				Nodes    []AccountHistoryEntry `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"watchHistory"`
		} `json:"user"`
	} `json:"data"`
}

const ratingsQuery = `
query GetReviewsHub($uuid: ID = "", $first: PaginationInt!, $after: String) {
  user(id: $uuid) {
    reviews(first: $first, after: $after) {
      nodes {
        ... on ActivityRating {
          metadataItem { id type }
          rating
        }
        ... on ActivityWatchRating {
          metadataItem { id type }
          rating
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

type ratingsResponse struct {
	Data struct {
		User struct {
			Reviews struct {
				Nodes []struct {
					MetadataItem struct {
						ID   string `json:"id"`
						Type string `json:"type"`
					} `json:"metadataItem"`
					Rating int `json:"rating"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"reviews"`
		} `json:"user"`
	} `json:"data"`
}

// AccountClient fetches watch history from the Plex account GraphQL API.
type AccountClient struct {
	Token    string
	UUID     string
	Username string
}

// RecentWatchActivities fetches the N most recent watch history entries and
// returns a map of metadataItem.ID → watch activity UUID. Used after batch
// scrobbles to find activity IDs for date correction.
func (c *AccountClient) RecentWatchActivities(count int) (map[string]string, error) {
	body, err := json.Marshal(graphqlRequest{
		Query: watchHistoryQuery,
		Variables: map[string]any{
			"uuid":          c.UUID,
			"first":         count,
			"skipUserState": true,
		},
		OperationName: "GetWatchHistoryHub",
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, accountHistoryEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Plex-Token", c.Token)
	req.Header.Set("X-Plex-Client-Identifier", "plexterbox")
	req.Header.Set("X-Plex-Product", "plexterbox")
	req.Header.Set("X-Plex-Platform", "Go")
	req.Header.Set("Origin", "https://app.plex.tv")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching recent activities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plex returned status %d: %s", resp.StatusCode, rawBody)
	}

	var result graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	m := make(map[string]string)
	for _, node := range result.Data.User.WatchHistory.Nodes {
		// Keep the first (most recent) activity per metadata item
		if _, exists := m[node.MetadataItem.ID]; !exists {
			m[node.MetadataItem.ID] = node.ID
		}
	}
	return m, nil
}

// AllWatchedMovies fetches movies watched in the past 30 days from the Plex account.
// A 3500ms cooldown is applied between requests.
func (c *AccountClient) AllWatchedMovies() ([]AccountHistoryEntry, error) {
	var all []AccountHistoryEntry
	var cursor *string
	requests := 0
	cutoff := time.Now().AddDate(0, 0, -60)

	for {
		vars := map[string]any{
			"uuid":          c.UUID,
			"first":         50,
			"skipUserState": true,
		}
		if cursor != nil {
			vars["after"] = *cursor
		}

		body, err := json.Marshal(graphqlRequest{
			Query:         watchHistoryQuery,
			Variables:     vars,
			OperationName: "GetWatchHistoryHub",
		})
		if err != nil {
			return nil, fmt.Errorf("marshalling request: %w", err)
		}

		req, err := http.NewRequest(http.MethodPost, accountHistoryEndpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Plex-Token", c.Token)
		req.Header.Set("X-Plex-Client-Identifier", "plexterbox")
		req.Header.Set("X-Plex-Product", "plexterbox")
		req.Header.Set("X-Plex-Platform", "Go")
		req.Header.Set("Origin", "https://app.plex.tv")

		resp, err := http.DefaultClient.Do(req)
		requests++
		log.Printf("[plex] history page %d fetched", requests)
		if err != nil {
			return nil, fmt.Errorf("fetching page: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("plex returned status %d: %s", resp.StatusCode, body)
		}

		var result graphqlResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		done := false
		for _, node := range result.Data.User.WatchHistory.Nodes {
			t, err := time.Parse(time.RFC3339, node.Date)
			if err == nil && t.Before(cutoff) {
				done = true
				break
			}
			if node.MetadataItem.Type == "MOVIE" {
				all = append(all, node)
			}
		}
		if done {
			break
		}

		if !result.Data.User.WatchHistory.PageInfo.HasNextPage {
			break
		}
		nextCursor := result.Data.User.WatchHistory.PageInfo.EndCursor
		cursor = &nextCursor
		time.Sleep(3500 * time.Millisecond)
	}

	return all, nil
}

// FetchAllRatings fetches all movie ratings and returns a map of metadataItem ID → rating (1-10).
func (c *AccountClient) FetchAllRatings() (map[string]int, error) {
	ratings := make(map[string]int)
	var cursor *string
	requests := 0

	for {
		vars := map[string]any{
			"uuid":  c.UUID,
			"first": 50,
		}
		if cursor != nil {
			vars["after"] = *cursor
		}

		body, err := json.Marshal(graphqlRequest{
			Query:         ratingsQuery,
			Variables:     vars,
			OperationName: "GetReviewsHub",
		})
		if err != nil {
			return nil, fmt.Errorf("marshalling ratings request: %w", err)
		}

		req, err := http.NewRequest(http.MethodPost, accountHistoryEndpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("building ratings request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Plex-Token", c.Token)
		req.Header.Set("X-Plex-Client-Identifier", "plexterbox")
		req.Header.Set("X-Plex-Product", "plexterbox")
		req.Header.Set("X-Plex-Platform", "Go")
		req.Header.Set("Origin", "https://app.plex.tv")

		resp, err := http.DefaultClient.Do(req)
		requests++
		log.Printf("[plex] ratings page %d fetched", requests)
		if err != nil {
			return nil, fmt.Errorf("fetching ratings page: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			rawBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("plex ratings returned status %d: %s", resp.StatusCode, rawBody)
		}

		var result ratingsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decoding ratings response: %w", err)
		}

		for _, node := range result.Data.User.Reviews.Nodes {
			if node.MetadataItem.Type != "MOVIE" || node.Rating == 0 {
				continue
			}
			// Keep the first (most recent) rating per movie
			if _, exists := ratings[node.MetadataItem.ID]; !exists {
				ratings[node.MetadataItem.ID] = node.Rating
			}
		}

		if !result.Data.User.Reviews.PageInfo.HasNextPage {
			break
		}
		nextCursor := result.Data.User.Reviews.PageInfo.EndCursor
		cursor = &nextCursor
		time.Sleep(3500 * time.Millisecond)
	}

	log.Printf("[plex] fetched %d movie ratings", len(ratings))
	return ratings, nil
}

const updateActivityDateMutation = `
    mutation updateActivityDate($id: ID!, $input: UpdateActivityInput!) {
  updateActivity(id: $id, input: $input) {
    id
  }
}
    `

// UpdateActivityDate changes the watch date of a Plex activity entry.
// activityID is the UUID from the watch history, newDate is ISO 8601 UTC.
func (c *AccountClient) UpdateActivityDate(activityID, newDate string) error {
	body, err := json.Marshal(graphqlRequest{
		Query: updateActivityDateMutation,
		Variables: map[string]any{
			"id": activityID,
			"input": map[string]any{
				"date": newDate,
			},
		},
		OperationName: "updateActivityDate",
	})
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, accountHistoryEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Plex-Token", c.Token)
	req.Header.Set("X-Plex-Client-Identifier", "plexterbox")
	req.Header.Set("X-Plex-Product", "plexterbox")
	req.Header.Set("X-Plex-Platform", "Go")
	req.Header.Set("Origin", "https://app.plex.tv")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plex returned status %d: %s", resp.StatusCode, rawBody)
	}

	return nil
}

