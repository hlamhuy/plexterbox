package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	plexterboxdb "plexterbox/db"
	"plexterbox/plex"
)

func handlePlexStatus(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	client := plexAccount
	plexMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if client != nil {
		json.NewEncoder(w).Encode(map[string]any{"connected": true, "username": client.Username})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"connected": false})
	}
}

func handlePlexLogout(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	plexAccount = nil
	plexMu.Unlock()

	persistSession()

	log.Println("[session] plex disconnected")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handlePlexOAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID := uuid.New().String()

	pin, authURL, err := plex.CreatePin(clientID)
	if err != nil {
		log.Printf("[plex] oauth start error: %v", err)
		http.Error(w, `{"error":"failed to create plex pin"}`, http.StatusInternalServerError)
		return
	}

	plexMu.Lock()
	plexPin = pin
	plexClientID = clientID
	plexMu.Unlock()

	log.Printf("[plex] oauth started: pin=%d, clientID=%s", pin.ID, clientID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"authUrl": authURL})
}

func handlePlexOAuthCheck(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	pin := plexPin
	clientID := plexClientID
	plexMu.Unlock()

	if pin == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no pending pin, start oauth first"})
		return
	}

	token, err := plex.CheckPin(pin.ID, pin.Code, clientID)
	if err != nil {
		log.Printf("[plex] oauth check error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
		return
	}

	// Got a token — fetch account info (UUID + username)
	info, err := plex.GetAccountInfo(token)
	if err != nil {
		log.Printf("[plex] get account error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "got token but failed to fetch account: " + err.Error()})
		return
	}

	plexMu.Lock()
	plexPin = nil
	plexAccount = &plex.AccountClient{Token: token, UUID: info.UUID, Username: info.Username}
	currentPlex := plexAccount
	plexMu.Unlock()

	persistSession()

	log.Printf("[plex] oauth complete: user=%s", currentPlex.Username)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"username": currentPlex.Username,
	})
}

type plexImportRequest struct {
	Films []struct {
		Title      string  `json:"title"`
		Year       int     `json:"year"`
		Rating     float64 `json:"rating"`     // 1-10 scale
		WatchedDate string `json:"watchedDate"` // YYYY-MM-DD from LB diary
	} `json:"films"`
}

func handlePlexImport(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	client := plexAccount
	clientID := plexClientID
	plexMu.Unlock()

	if client == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "plex not connected"})
		return
	}

	var req plexImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if len(req.Films) == 0 {
		http.Error(w, `{"error":"no films to import"}`, http.StatusBadRequest)
		return
	}

	discover := &plex.DiscoverClient{
		Token:    client.Token,
		ClientID: clientID,
	}

	filmStatuses := make([]string, len(req.Films))
	imported := 0

	// Track scrobbled films that need date correction: metadataID → desired date
	type datefix struct {
		metadataID  string
		watchedDate string
		title       string
	}
	var dateFixes []datefix

	for i, film := range req.Films {
		// Search for the movie on Discover
		results, err := discover.SearchMovie(film.Title, film.Year)
		if err != nil {
			log.Printf("[plex-import] search error for %q (%d): %v", film.Title, film.Year, err)
			filmStatuses[i] = "not_found"
			continue
		}

		// Find best match by title+year
		metadataID := ""
		for _, r := range results {
			if r.Year == film.Year {
				metadataID = r.RatingKey
				break
			}
		}
		if metadataID == "" && len(results) > 0 {
			metadataID = results[0].RatingKey
		}
		if metadataID == "" {
			log.Printf("[plex-import] no results for %q (%d)", film.Title, film.Year)
			filmStatuses[i] = "not_found"
			continue
		}

		// Mark as watched
		if err := discover.Scrobble(metadataID); err != nil {
			log.Printf("[plex-import] scrobble error for %q: %v", film.Title, err)
			filmStatuses[i] = "error"
			continue
		}

		// Rate if applicable (1-10 scale)
		if film.Rating > 0 {
			rating := int(film.Rating)
			if rating < 1 {
				rating = 1
			} else if rating > 10 {
				rating = 10
			}
			if err := discover.Rate(metadataID, rating); err != nil {
				log.Printf("[plex-import] rate error for %q: %v", film.Title, err)
			}
		}

		filmStatuses[i] = "imported"
		imported++
		log.Printf("[plex-import] %q (%d) → %s", film.Title, film.Year, metadataID)

		if film.WatchedDate != "" {
			dateFixes = append(dateFixes, datefix{metadataID, film.WatchedDate, film.Title})
		}
	}

	// Batch date correction: fetch recent watch history to get activity IDs,
	// then update dates for each scrobbled film.
	if len(dateFixes) > 0 {
		log.Printf("[plex-import] correcting dates for %d films, waiting for Plex to propagate...", len(dateFixes))
		time.Sleep(3 * time.Second)

		activities, err := client.RecentWatchActivities(len(dateFixes) + 10)
		if err != nil {
			log.Printf("[plex-import] failed to fetch recent activities: %v", err)
		} else {
			for _, fix := range dateFixes {
				activityID, ok := activities[fix.metadataID]
				if !ok {
					log.Printf("[plex-import] no watch activity found for %q (metadata %s)", fix.title, fix.metadataID)
					continue
				}
				utcDate := fix.watchedDate + "T12:00:00.000Z"
				if err := client.UpdateActivityDate(activityID, utcDate); err != nil {
					log.Printf("[plex-import] failed to update date for %q: %v", fix.title, err)
				} else {
					log.Printf("[plex-import] corrected date for %q → %s", fix.title, fix.watchedDate)
				}
			}
		}
	}

	notFound := 0
	for _, s := range filmStatuses {
		if s == "not_found" {
			notFound++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"filmStatuses": filmStatuses,
		"imported":     imported,
		"skipped":      len(req.Films) - imported - notFound,
		"total":        len(req.Films),
	})
}

type editPlexDateRequest struct {
	ActivityID string `json:"activityId"`
	Date       string `json:"date"` // YYYY-MM-DD
}

func handleEditPlexDate(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	client := plexAccount
	plexMu.Unlock()

	if client == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "plex not connected"})
		return
	}

	var req editPlexDateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.ActivityID == "" || req.Date == "" {
		http.Error(w, `{"error":"activityId and date are required"}`, http.StatusBadRequest)
		return
	}

	// Convert YYYY-MM-DD to UTC ISO 8601 (noon UTC to avoid timezone edge cases)
	utcDate := req.Date + "T12:00:00.000Z"

	if err := client.UpdateActivityDate(req.ActivityID, utcDate); err != nil {
		log.Printf("[edit-date] plex update error for %s: %v", req.ActivityID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to update date: " + err.Error()})
		return
	}

	// Update our local DB too
	localDate := req.Date + "T12:00:00.000"
	n, err := plexterboxdb.UpdatePlexWatchDate(appDB, req.ActivityID, localDate)
	if err != nil {
		log.Printf("[edit-date] db update error: %v", err)
	} else {
		log.Printf("[edit-date] updated %d DB row(s) for activity %s → %s", n, req.ActivityID, req.Date)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
