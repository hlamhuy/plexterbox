package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	plexterboxdb "plexterbox/db"
	"plexterbox/letterboxd"
	"plexterbox/plex"
	"plexterbox/session"
)

var (
	lbClient  *letterboxd.Client
	lbPending *letterboxd.PendingLogin
	lbMu      sync.Mutex

	plexPin      *plex.Pin
	plexClientID string
	plexAccount  *plex.AccountClient // stored server-side after OAuth
	plexMu       sync.Mutex

	appDB *sql.DB
)

// statusRecorder wraps ResponseWriter to capture the written HTTP status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("[http] %s %s → %d %s", r.Method, r.URL.Path, rec.status, fmt.Sprintf("(%s)", time.Since(start).Round(time.Millisecond)))
	})
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)

	// Port: -port flag → PORT env var → default 12349
	portFlag := flag.String("port", "", "port to listen on (overrides PORT env var)")
	flag.Parse()
	port := "12349"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	if *portFlag != "" {
		port = *portFlag
	}
	addr := ":" + port

	// Open (or create) the SQLite database.
	var err error
	appDB, err = plexterboxdb.Open()
	if err != nil {
		log.Fatalf("[db] failed to open: %v", err)
	}

	// Restore any previously saved session from disk
	saved := session.Load()
	if saved.PlexToken != "" && saved.PlexUUID != "" {
		plexAccount = &plex.AccountClient{Token: saved.PlexToken, UUID: saved.PlexUUID, Username: saved.PlexUsername}
		plexClientID = uuid.New().String()
		log.Printf("[session] restored plex session for %s", saved.PlexUsername)
	}
	if saved.LbCookies != "" && saved.LbUsername != "" {
		ua := saved.LbUserAgent
		if ua == "" {
			ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
		}
		lbClient = letterboxd.NewClient(ua, saved.LbCookies, saved.LbCSRFToken)
		lbClient.Username = saved.LbUsername
		log.Printf("[session] restored letterboxd session for %s", saved.LbUsername)
	}

	mux := http.NewServeMux()

	// Plex handlers
	mux.HandleFunc("GET /api/plex/status", handlePlexStatus)
	mux.HandleFunc("POST /api/plex/logout", handlePlexLogout)
	mux.HandleFunc("POST /api/plex/oauth/start", handlePlexOAuthStart)
	mux.HandleFunc("GET /api/plex/oauth/check", handlePlexOAuthCheck)
	mux.HandleFunc("POST /api/plex/import", handlePlexImport)
	mux.HandleFunc("PUT /api/plex/activity/date", handleEditPlexDate)

	// Letterboxd handlers
	mux.HandleFunc("POST /api/letterboxd/login", handleLetterboxdLogin)
	mux.HandleFunc("POST /api/letterboxd/totp", handleLetterboxdTOTP)
	mux.HandleFunc("GET /api/letterboxd/status", handleLetterboxdStatus)
	mux.HandleFunc("POST /api/letterboxd/logout", handleLetterboxdLogout)
	mux.HandleFunc("POST /api/letterboxd/import", handleLetterboxdImport)

	// Sync & data handlers
	mux.HandleFunc("POST /api/sync", handleSync)
	mux.HandleFunc("GET /api/movies", handleMovies)
	mux.HandleFunc("DELETE /api/db", handleDeleteDB)

	// Auto-sync
	mux.HandleFunc("GET /api/autosync", handleGetAutoSync)
	mux.HandleFunc("PUT /api/autosync", handleSetAutoSync)

	// Serve embedded frontend for all non-API routes.
	// In dev, Vite handles this instead (npm run dev).
	mux.Handle("/", spaHandler())

	log.Printf("[startup] plexterbox listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, loggingMiddleware(corsMiddleware(mux))))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
