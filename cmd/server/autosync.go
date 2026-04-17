package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// AutoSyncConfig holds the current auto-sync settings.
type AutoSyncConfig struct {
	Mode      string `json:"mode"`      // "disabled", "safe", "fast"
	Interval  string `json:"interval"`  // "5m", "15m", "3h", etc.
	Direction string `json:"direction"` // "full", "plexToLb", "lbToPlex"
}

var (
	autoSyncMu     sync.Mutex
	autoSyncCfg    = AutoSyncConfig{Mode: "disabled", Interval: "15m", Direction: "full"}
	autoSyncLastAt string
	autoSyncStop   chan struct{}
)

func handleGetAutoSync(w http.ResponseWriter, r *http.Request) {
	autoSyncMu.Lock()
	cfg := autoSyncCfg
	lastAt := autoSyncLastAt
	autoSyncMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"mode":       cfg.Mode,
		"interval":   cfg.Interval,
		"direction":  cfg.Direction,
		"lastSyncAt": lastAt,
	})
}

func handleSetAutoSync(w http.ResponseWriter, r *http.Request) {
	var cfg AutoSyncConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	switch cfg.Mode {
	case "disabled", "safe", "fast":
	default:
		http.Error(w, `{"error":"invalid mode"}`, http.StatusBadRequest)
		return
	}

	applyAutoSyncConfig(cfg)

	autoSyncMu.Lock()
	autoSyncCfg = cfg
	autoSyncMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// applyAutoSyncConfig stops any running ticker and starts a new one for safe mode.
func applyAutoSyncConfig(cfg AutoSyncConfig) {
	autoSyncMu.Lock()
	if autoSyncStop != nil {
		close(autoSyncStop)
		autoSyncStop = nil
	}
	autoSyncMu.Unlock()

	if cfg.Mode != "safe" {
		return
	}

	dur, err := time.ParseDuration(cfg.Interval)
	if err != nil || dur < time.Minute {
		log.Printf("[autosync] invalid interval %q, defaulting to 15m", cfg.Interval)
		dur = 15 * time.Minute
	}

	stop := make(chan struct{})
	autoSyncMu.Lock()
	autoSyncStop = stop
	autoSyncMu.Unlock()

	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()
		log.Printf("[autosync] safe mode started — interval=%s direction=%s", cfg.Interval, cfg.Direction)
		for {
			select {
			case <-ticker.C:
				runSafeModeJob(cfg.Direction)
			case <-stop:
				log.Println("[autosync] stopped")
				return
			}
		}
	}()
}

// runSafeModeJob fetches data from the configured platforms and upserts into the DB.
func runSafeModeJob(direction string) {
	log.Printf("[autosync] running job, direction=%s", direction)

	plexMu.Lock()
	pc := plexAccount
	plexMu.Unlock()

	lbMu.Lock()
	lc := lbClient
	lbMu.Unlock()

	if (direction == "full" || direction == "plexToLb") && pc != nil {
		if err := plexFetchUpsert(pc); err != nil {
			log.Printf("[autosync] plex error: %v", err)
		}
	}

	if (direction == "full" || direction == "lbToPlex") && lc != nil && lc.Username != "" {
		if err := lbFetchUpsert(lc); err != nil {
			log.Printf("[autosync] lb error: %v", err)
		}
	}

	autoSyncMu.Lock()
	autoSyncLastAt = time.Now().UTC().Format(time.RFC3339)
	autoSyncMu.Unlock()
	log.Println("[autosync] job complete")
}
