package letterboxd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type loginResponse struct {
	Result   string   `json:"result"`
	Messages []string `json:"messages"`
	CSRF     string   `json:"csrf"`
}

// ErrTOTPRequired is returned when the account has 2FA enabled.
var ErrTOTPRequired = fmt.Errorf("totp required")

// PendingLogin holds the state between the password step and the TOTP step.
type PendingLogin struct {
	HTTPClient *http.Client
	UserAgent  string
	CSRF       string
	Username   string
	Password   string
}

// Login authenticates with Letterboxd using username and password.
// If the account has 2FA, it returns (nil, pendingLogin, ErrTOTPRequired).
func Login(username, password string) (*Client, *PendingLogin, error) {
	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Transport: NewChromeTransport(),
		Jar:       jar,
		Timeout:   30 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://letterboxd.com/sign-in/", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("building sign-in request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching sign-in page: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("sign-in page returned status %d", resp.StatusCode)
	}

	u, _ := url.Parse("https://letterboxd.com")
	csrf := ""
	for _, c := range jar.Cookies(u) {
		if c.Name == "com.xk72.webparts.csrf" {
			csrf = c.Value
		}
	}
	if csrf == "" {
		return nil, nil, fmt.Errorf("no CSRF cookie received")
	}

	loginData := url.Values{}
	loginData.Set("__csrf", csrf)
	loginData.Set("username", username)
	loginData.Set("password", password)
	loginData.Set("remember", "true")

	loginReq, err := http.NewRequest("POST", "https://letterboxd.com/user/login.do", strings.NewReader(loginData.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("building login request: %w", err)
	}
	loginReq.Header.Set("User-Agent", userAgent)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	loginReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	loginReq.Header.Set("Origin", "https://letterboxd.com")
	loginReq.Header.Set("Referer", "https://letterboxd.com/sign-in/")

	loginResp, err := httpClient.Do(loginReq)
	if err != nil {
		return nil, nil, fmt.Errorf("login request failed: %w", err)
	}
	defer loginResp.Body.Close()

	body, _ := io.ReadAll(loginResp.Body)
	log.Printf("[lb-login] POST status: %d", loginResp.StatusCode)

	var result loginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil, fmt.Errorf("decoding login response (body=%s): %w", string(body), err)
	}

	if result.CSRF != "" {
		csrf = result.CSRF
	}

	if result.Result == "totpRequired" {
		log.Printf("[lb-login] TOTP required, returning pending session")
		return nil, &PendingLogin{
			HTTPClient: httpClient,
			UserAgent:  userAgent,
			CSRF:       csrf,
			Username:   username,
			Password:   password,
		}, ErrTOTPRequired
	}

	if result.Result != "success" {
		msg := "login failed"
		if len(result.Messages) > 0 {
			msg = result.Messages[0]
		}
		return nil, nil, fmt.Errorf("%s", msg)
	}
	log.Printf("[lb-login] login successful (no 2FA)")

	return buildClient(httpClient, userAgent, csrf), nil, nil
}

// SubmitTOTP completes login for accounts with 2FA enabled.
func (p *PendingLogin) SubmitTOTP(code string) (*Client, error) {
	totpData := url.Values{}
	totpData.Set("__csrf", p.CSRF)
	totpData.Set("username", p.Username)
	totpData.Set("password", p.Password)
	totpData.Set("authenticationCode", code)

	req, err := http.NewRequest("POST", "https://letterboxd.com/user/login.do", strings.NewReader(totpData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building TOTP request: %w", err)
	}
	req.Header.Set("User-Agent", p.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", "https://letterboxd.com")
	req.Header.Set("Referer", "https://letterboxd.com/sign-in/")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TOTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[lb-login] TOTP POST status: %d", resp.StatusCode)

	var result loginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding TOTP response (body=%s): %w", string(body), err)
	}

	csrf := p.CSRF
	if result.CSRF != "" {
		csrf = result.CSRF
	}

	if result.Result != "success" {
		msg := "invalid code"
		if len(result.Messages) > 0 {
			msg = result.Messages[0]
		}
		return nil, fmt.Errorf("%s", msg)
	}
	log.Printf("[lb-login] TOTP login successful")

	return buildClient(p.HTTPClient, p.UserAgent, csrf), nil
}

func buildClient(httpClient *http.Client, userAgent, csrf string) *Client {
	u, _ := url.Parse("https://letterboxd.com")
	var cookieParts []string
	for _, c := range httpClient.Jar.Cookies(u) {
		cookieParts = append(cookieParts, c.Name+"="+c.Value)
	}

	return &Client{
		HTTPClient: httpClient,
		UserAgent:  userAgent,
		Cookies:    strings.Join(cookieParts, "; "),
		CSRFToken:  csrf,
	}
}
