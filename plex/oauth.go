package plex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	plexPinsURL   = "https://plex.tv/api/v2/pins"
	plexAuthURL   = "https://app.plex.tv/auth#!?clientID=%s&code=%s&context%%5Bdevice%%5D%%5Bproduct%%5D=Plexterboxd"
	plexProductName = "Plexterboxd"
)

// Pin represents a Plex OAuth pin response.
type Pin struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
	ClientID  string `json:"clientIdentifier"`
}

// CreatePin generates a new Plex OAuth pin. Returns the pin and the auth URL for the user.
func CreatePin(clientID string) (*Pin, string, error) {
	data := url.Values{}
	data.Set("strong", "true")
	data.Set("X-Plex-Product", plexProductName)
	data.Set("X-Plex-Client-Identifier", clientID)

	req, err := http.NewRequest("POST", plexPinsURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("building pin request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pin request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, "", fmt.Errorf("pin request returned %d: %s", resp.StatusCode, string(body))
	}

	var pin Pin
	if err := json.Unmarshal(body, &pin); err != nil {
		return nil, "", fmt.Errorf("decoding pin response: %w", err)
	}
	pin.ClientID = clientID

	authURL := fmt.Sprintf(plexAuthURL, clientID, pin.Code)
	return &pin, authURL, nil
}

// CheckPin polls the pin to see if the user has authenticated.
// Returns the auth token if ready, empty string if still pending.
func CheckPin(pinID int, code, clientID string) (string, error) {
	checkURL := fmt.Sprintf("%s/%d", plexPinsURL, pinID)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return "", fmt.Errorf("building check request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Client-Identifier", clientID)
	q := req.URL.Query()
	q.Set("code", code)
	req.URL.RawQuery = q.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("check request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check returned %d: %s", resp.StatusCode, string(body))
	}

	var pin Pin
	if err := json.Unmarshal(body, &pin); err != nil {
		return "", fmt.Errorf("decoding check response: %w", err)
	}

	return pin.AuthToken, nil
}

// AccountInfo holds basic Plex account details.
type AccountInfo struct {
	UUID     string
	Username string
}

// GetAccountInfo fetches the user's UUID and username using an auth token.
func GetAccountInfo(token string) (AccountInfo, error) {
	req, err := http.NewRequest("GET", "https://plex.tv/api/v2/user", nil)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("building user request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Client-Identifier", "plexterboxd")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("user request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return AccountInfo{}, fmt.Errorf("user request returned %d: %s", resp.StatusCode, string(body))
	}

	var user struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return AccountInfo{}, fmt.Errorf("decoding user response: %w", err)
	}

	return AccountInfo{UUID: user.UUID, Username: user.Username}, nil
}
