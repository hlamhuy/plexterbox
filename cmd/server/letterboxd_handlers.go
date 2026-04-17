package main

import (
	"encoding/json"
	"log"
	"net/http"

	"plexterbox/letterboxd"
)

type letterboxdLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type totpRequest struct {
	Code string `json:"code"`
}

type importRequest struct {
	Films []letterboxd.ImportFilm `json:"films"`
}

func handleLetterboxdLogin(w http.ResponseWriter, r *http.Request) {
	var req letterboxdLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"username and password are required"}`, http.StatusBadRequest)
		return
	}

	client, pending, err := letterboxd.Login(req.Username, req.Password)
	if err == letterboxd.ErrTOTPRequired {
		lbMu.Lock()
		lbPending = pending
		lbMu.Unlock()

		log.Printf("[lb] %s requires TOTP", req.Username)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "totp_required"})
		return
	}
	if err != nil {
		log.Printf("[lb] login error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	lbMu.Lock()
	client.Username = req.Username
	lbClient = client
	lbPending = nil
	lbMu.Unlock()

	persistSession()

	log.Printf("[lb] login successful for %s", req.Username)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "username": req.Username})
}

func handleLetterboxdTOTP(w http.ResponseWriter, r *http.Request) {
	var req totpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	lbMu.Lock()
	pending := lbPending
	lbMu.Unlock()

	if pending == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no pending login, start over"})
		return
	}

	client, err := pending.SubmitTOTP(req.Code)
	if err != nil {
		log.Printf("[lb] TOTP error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	lbMu.Lock()
	client.Username = pending.Username
	lbClient = client
	lbPending = nil
	lbMu.Unlock()

	persistSession()

	log.Println("[lb] TOTP login successful")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLetterboxdStatus(w http.ResponseWriter, r *http.Request) {
	lbMu.Lock()
	client := lbClient
	lbMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if client != nil {
		json.NewEncoder(w).Encode(map[string]any{"connected": true, "username": client.Username})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"connected": false})
	}
}

func handleLetterboxdLogout(w http.ResponseWriter, r *http.Request) {
	lbMu.Lock()
	lbClient = nil
	lbMu.Unlock()

	persistSession()

	log.Println("[session] letterboxd disconnected")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLetterboxdImport(w http.ResponseWriter, r *http.Request) {
	lbMu.Lock()
	client := lbClient
	lbMu.Unlock()

	if client == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not logged in to letterboxd"})
		return
	}

	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Films) == 0 {
		http.Error(w, `{"error":"no films to import"}`, http.StatusBadRequest)
		return
	}

	log.Printf("[import] matching %d films...", len(req.Films))
	matched, err := client.MatchFilms(req.Films)
	if err != nil {
		log.Printf("[import] match error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "matching failed: " + err.Error()})
		return
	}

	// Build per-film status: default to "not_found"
	filmStatuses := make([]string, len(req.Films))
	for i := range filmStatuses {
		filmStatuses[i] = "not_found"
	}

	var toSave []letterboxd.MatchedFilm
	for _, m := range matched {
		if m.Index < len(filmStatuses) {
			if m.ShouldImport {
				filmStatuses[m.Index] = "imported"
				toSave = append(toSave, m)
			} else {
				filmStatuses[m.Index] = "duplicate"
			}
		}
	}

	imported := 0
	if len(toSave) > 0 {
		log.Printf("[import] saving %d films (%d skipped as duplicates)...", len(toSave), len(matched)-len(toSave))
		result, err := client.SaveImport(toSave)
		if err != nil {
			log.Printf("[import] save error: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "save failed: " + err.Error()})
			return
		}
		imported = result.Success
	}

	skipped := len(matched) - len(toSave)
	log.Printf("[import] done: %d imported, %d duplicates skipped, %d not found", imported, skipped, len(req.Films)-len(matched))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"filmStatuses": filmStatuses,
		"imported":     imported,
		"skipped":      skipped,
		"total":        len(req.Films),
	})
}
