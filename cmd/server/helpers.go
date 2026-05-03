package main

import (
	"fmt"
	"log"
	"time"

	plexterboxdb "plexterbox/db"
	"plexterbox/letterboxd"
	"plexterbox/plex"
	"plexterbox/session"
)

// plexFetchUpsert fetches Plex watch history + ratings and upserts them into the DB.
// stopAtID halts pagination at that activity ID (autosync); pass "" for a full 60-day fetch.
func plexFetchUpsert(client *plex.AccountClient, stopAtID string) error {
	entries, err := client.AllWatchedMovies(stopAtID)
	if err != nil {
		return fmt.Errorf("fetch history: %w", err)
	}

	ratings, err := client.FetchAllRatings()
	if err != nil {
		log.Printf("[plex] fetch ratings (non-fatal): %v", err)
	} else {
		for i, e := range entries {
			if r, ok := ratings[e.MetadataItem.ID]; ok {
				entries[i].Rating = r
			}
		}
	}

	dbInputs := make([]plexterboxdb.PlexEntryInput, 0, len(entries))
	for _, e := range entries {
		watchedAt := e.Date
		if t, err := time.Parse(time.RFC3339Nano, e.Date); err == nil {
			watchedAt = t.Local().Format("2006-01-02T15:04:05.000")
		}
		dbInputs = append(dbInputs, plexterboxdb.PlexEntryInput{
			ActivityID: e.ID,
			RatingKey:  e.RatingKey(),
			Title:      e.MetadataItem.Title,
			Year:       e.MetadataItem.Year,
			WatchedAt:  watchedAt,
			Rating:     float64(e.Rating),
		})
	}

	if err := plexterboxdb.UpsertPlexEntries(appDB, dbInputs); err != nil {
		return fmt.Errorf("db upsert: %w", err)
	}
	log.Printf("[plex] wrote %d entries", len(dbInputs))
	return nil
}

// lbFetchUpsert fetches the Letterboxd diary and upserts entries into the DB.
// stopAtID halts pagination at that viewing ID (autosync); pass "" for a full 60-day fetch.
func lbFetchUpsert(client *letterboxd.Client, stopAtID string) error {
	entries, err := client.FetchDiary(client.Username, stopAtID)
	if err != nil {
		return fmt.Errorf("fetch diary: %w", err)
	}

	dbInputs := make([]plexterboxdb.DiaryEntryInput, 0, len(entries))
	for _, e := range entries {
		year := 0
		fmt.Sscanf(e.Year, "%d", &year)
		dbInputs = append(dbInputs, plexterboxdb.DiaryEntryInput{
			ViewingID: e.ViewingID,
			Slug:      e.Slug,
			Title:     e.Title,
			Year:      year,
			WatchedOn: e.WatchedOn,
			Rating:    float64(e.Rating),
			Rewatch:   e.Rewatch,
		})
	}

	if err := plexterboxdb.UpsertDiaryEntries(appDB, dbInputs); err != nil {
		return fmt.Errorf("db upsert: %w", err)
	}
	log.Printf("[lb] wrote %d diary entries", len(dbInputs))
	return nil
}

// persistSession writes the current Plex + LB credentials to disk.
func persistSession() {
	plexMu.Lock()
	pc := plexAccount
	plexMu.Unlock()

	lbMu.Lock()
	lc := lbClient
	lbMu.Unlock()

	var sd session.Data
	if pc != nil {
		sd.PlexToken = pc.Token
		sd.PlexUUID = pc.UUID
		sd.PlexUsername = pc.Username
	}
	if lc != nil {
		sd.LbUsername  = lc.Username
		sd.LbCookies   = lc.Cookies
		sd.LbCSRFToken = lc.CSRFToken
		sd.LbUserAgent = lc.UserAgent
	}
	session.Save(sd)
}
