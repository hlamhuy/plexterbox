package main

import (
	"encoding/json"
	"log"
	"net/http"

	plexterboxdb "plexterbox/db"
)

func handleSync(w http.ResponseWriter, r *http.Request) {
	plexMu.Lock()
	pc := plexAccount
	plexMu.Unlock()

	lbMu.Lock()
	lc := lbClient
	lbMu.Unlock()

	if pc == nil && lc == nil {
		http.Error(w, `{"error":"no platforms connected"}`, http.StatusBadRequest)
		return
	}

	var syncErrors []string

	if pc != nil {
		if err := plexFetchUpsert(pc, ""); err != nil {
			log.Printf("[sync] plex error: %v", err)
			syncErrors = append(syncErrors, "plex: "+err.Error())
		}
	}

	if lc != nil && lc.Username != "" {
		if err := lbFetchUpsert(lc, ""); err != nil {
			log.Printf("[sync] lb error: %v", err)
			syncErrors = append(syncErrors, "letterboxd: "+err.Error())
		}
	}

	// Return unified data from DB
	events, err := plexterboxdb.AllWatchEvents(appDB)
	if err != nil {
		log.Printf("[sync] query watch events error: %v", err)
		http.Error(w, `{"error":"failed to query movies"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"events":        events,
		"plexFetchedAt": plexterboxdb.LastFetchedAt(appDB, "plex"),
		"lbFetchedAt":   plexterboxdb.LastFetchedAt(appDB, "letterboxd"),
		"errors":        syncErrors,
	})
}

func handleMovies(w http.ResponseWriter, r *http.Request) {
	events, err := plexterboxdb.AllWatchEvents(appDB)
	if err != nil {
		log.Printf("[db] query watch events error: %v", err)
		http.Error(w, `{"error":"failed to query movies"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"events":        events,
		"plexFetchedAt": plexterboxdb.LastFetchedAt(appDB, "plex"),
		"lbFetchedAt":   plexterboxdb.LastFetchedAt(appDB, "letterboxd"),
	})
}

func handleDeleteDB(w http.ResponseWriter, r *http.Request) {
	if err := plexterboxdb.ClearAll(appDB); err != nil {
		log.Printf("[db] clear all error: %v", err)
		http.Error(w, `{"error":"failed to clear database"}`, http.StatusInternalServerError)
		return
	}
	log.Println("[db] all data cleared")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
