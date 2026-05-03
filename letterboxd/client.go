package letterboxd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// NewChromeTransport creates an http.RoundTripper that spoofs Chrome's TLS
// fingerprint and negotiates HTTP/2, matching real Chrome browser behaviour.
// Forcing HTTP/1.1 (the old approach) is a detectable Cloudflare signal.
func NewChromeTransport() http.RoundTripper {
	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			dialer := net.Dialer{Timeout: 10 * time.Second}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, fmt.Errorf("dial: %w", err)
			}

			// Use Chrome's full TLS spec including h2 in ALPN.
			spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("utls spec: %w", err)
			}

			tlsConn := utls.UClient(conn, &utls.Config{
				ServerName: host,
			}, utls.HelloCustom)

			if err := tlsConn.ApplyPreset(&spec); err != nil {
				conn.Close()
				return nil, fmt.Errorf("apply preset: %w", err)
			}

			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, fmt.Errorf("tls handshake: %w", err)
			}

			return tlsConn, nil
		},
	}
}

// Client handles communication with Letterboxd using a Chrome TLS fingerprint.
type Client struct {
	HTTPClient *http.Client
	UserAgent  string
	Cookies    string
	CSRFToken  string
	Username   string
}

// NewClient creates a Letterboxd client with Chrome TLS fingerprint and the given session cookies.
func NewClient(userAgent, cookies, csrfToken string) *Client {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://letterboxd.com")

	// Parse cookie string into individual cookies
	header := http.Header{}
	header.Add("Cookie", cookies)
	parsedReq := http.Request{Header: header}
	for _, c := range parsedReq.Cookies() {
		jar.SetCookies(u, []*http.Cookie{c})
	}

	return &Client{
		HTTPClient: &http.Client{
			Transport: NewChromeTransport(),
			Jar:       jar,
			Timeout:   30 * time.Second,
		},
		UserAgent: userAgent,
		Cookies:   cookies,
		CSRFToken: csrfToken,
	}
}

// TestConnection makes a simple request to verify the CF clearance works.
func (c *Client) TestConnection() (int, string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://letterboxd.com/", nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Cookie", c.Cookies)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Return first 500 chars to see what we got
	preview := string(body)
	if len(preview) > 500 {
		preview = preview[:500]
	}

	return resp.StatusCode, preview, nil
}
