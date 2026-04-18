package session

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Data holds all auth credentials persisted to disk.
// Nothing leaves the local machine — this file is written only to the user's
// config directory and is never transmitted anywhere.
type Data struct {
	PlexToken    string `json:"plexToken,omitempty"`
	PlexUUID     string `json:"plexUUID,omitempty"`
	PlexUsername string `json:"plexUsername,omitempty"`
	LbUsername   string `json:"lbUsername,omitempty"`
	LbCookies    string `json:"lbCookies,omitempty"`
	LbCSRFToken  string `json:"lbCsrfToken,omitempty"`
	LbUserAgent  string `json:"lbUserAgent,omitempty"`
}

func configPath() (string, error) {
	var appDir string
	if d := os.Getenv("DATA_DIR"); d != "" {
		appDir = d
	} else {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		appDir = filepath.Join(dir, "plexterbox")
	}
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "session.json"), nil
}

// Load reads persisted session data from disk.
// Returns an empty Data struct if no file exists yet.
func Load() Data {
	path, err := configPath()
	if err != nil {
		log.Printf("[session] cannot determine config path: %v", err)
		return Data{}
	}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Data{}
	}
	if err != nil {
		log.Printf("[session] cannot read session file: %v", err)
		return Data{}
	}

	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		log.Printf("[session] cannot parse session file: %v", err)
		return Data{}
	}

	log.Printf("[session] loaded from %s", path)
	return d
}

// Save writes session data to disk with restrictive permissions (owner-only).
func Save(d Data) {
	path, err := configPath()
	if err != nil {
		log.Printf("[session] cannot determine config path: %v", err)
		return
	}

	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		log.Printf("[session] cannot marshal session: %v", err)
		return
	}

	if err := os.WriteFile(path, b, 0600); err != nil {
		log.Printf("[session] cannot write session file: %v", err)
		return
	}

	log.Printf("[session] saved to %s", path)
}

// Clear deletes the session file from disk.
func Clear() {
	path, err := configPath()
	if err != nil {
		return
	}
	os.Remove(path)
}
