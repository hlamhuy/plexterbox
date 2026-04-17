package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// -------------------------------------------------------------------
// Input types (used by the server to pass fetched data into the DB)
// Defined here to keep plex/ and letterboxd/ packages decoupled from db/.
// -------------------------------------------------------------------

// PlexEntryInput is a single watch event from the Plex account GraphQL API.
type PlexEntryInput struct {
	ActivityID string  // community.plex.tv watch-event UUID
	RatingKey  string  // Discover metadata hex ID (e.g. "64dd290c84713e6f8ba2874b")
	Title      string
	Year       int
	WatchedAt  string  // ISO 8601, e.g. "2024-07-13T20:11:00.000Z"
	Rating     float64 // 0 = no rating; 1-10 scale
}

// DiaryEntryInput is a single entry from the Letterboxd diary scraper.
type DiaryEntryInput struct {
	Slug      string
	Title     string
	Year      int
	WatchedOn string  // YYYY-MM-DD
	Rating    float64 // 0 = no rating; 1-10 scale
	Rewatch   bool
}

// -------------------------------------------------------------------
// Output type returned to the frontend via /api/movies
// -------------------------------------------------------------------

// WatchEvent is the unified DB row serialised to JSON for the frontend.
type WatchEvent struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year"`

	// Resolved display values (LB trumps Plex)
	WatchedOn string  `json:"watchedOn"` // most recent / preferred date
	Rating    float64 `json:"rating"`    // 0 = none
	Rewatch   bool    `json:"rewatch"`

	// Plex-specific
	PlexRatingKey  string  `json:"plexRatingKey,omitempty"`
	PlexActivityID string  `json:"plexActivityId,omitempty"`
	PlexWatchedAt  string  `json:"plexWatchedAt,omitempty"`
	PlexRating     float64 `json:"plexRating,omitempty"`

	// Letterboxd-specific
	LbSlug      string  `json:"lbSlug,omitempty"`
	LbWatchedOn string  `json:"lbWatchedOn,omitempty"`
	LbRating    float64 `json:"lbRating,omitempty"`
	LbRewatch   bool    `json:"lbRewatch,omitempty"`

	// Sync state
	InPlex         bool   `json:"inPlex"`
	InLb           bool   `json:"inLb"`
	PlexSyncStatus string `json:"plexSyncStatus,omitempty"`
	LbSyncStatus   string `json:"lbSyncStatus,omitempty"`
}

// -------------------------------------------------------------------
// Open / initialise
// -------------------------------------------------------------------

func dbPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "plexterbox")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "plexterbox.db"), nil
}

// Open opens (or creates) the SQLite database and runs the schema migration.
func Open() (*sql.DB, error) {
	path, err := dbPath()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Single writer is fine for this app.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("[db] opened at %s", path)
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS watch_events (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			title            TEXT NOT NULL,
			year             INTEGER,
			tmdb_id          TEXT,

			-- Plex
			plex_rating_key  TEXT,
			plex_activity_id TEXT UNIQUE,
			plex_watched_at  TEXT,
			plex_rating      REAL,
			plex_rewatch     INTEGER NOT NULL DEFAULT 0,

			-- Letterboxd
			lb_slug          TEXT,
			lb_watched_on    TEXT,
			lb_rating        REAL,
			lb_rewatch       INTEGER NOT NULL DEFAULT 0,

			-- Sync state
			in_plex          INTEGER NOT NULL DEFAULT 0,
			in_lb            INTEGER NOT NULL DEFAULT 0,
			plex_sync_status TEXT,
			lb_sync_status   TEXT,

			imported_to_plex_at TEXT,
			imported_to_lb_at   TEXT,

			UNIQUE (lb_slug, lb_watched_on)
		);

		CREATE TABLE IF NOT EXISTS sync_log (
			source          TEXT PRIMARY KEY,
			last_fetched_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add plex_rewatch column for existing databases.
	_, _ = db.Exec(`ALTER TABLE watch_events ADD COLUMN plex_rewatch INTEGER NOT NULL DEFAULT 0`)

	return nil
}

// -------------------------------------------------------------------
// Upsert helpers
// -------------------------------------------------------------------

// UpsertPlexEntries writes Plex watch-history entries into the DB.
// Keyed on plex_activity_id (unique per watch event).
// If a matching LB-only row exists (same title + same date, no Plex data yet),
// it is merged in-place instead of creating a duplicate row.
func UpsertPlexEntries(db *sql.DB, entries []PlexEntryInput) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Try to merge into an existing LB-only row with the same title and watch date.
	mergeStmt, err := tx.Prepare(`
		UPDATE watch_events SET
			plex_rating_key  = ?,
			plex_activity_id = ?,
			plex_watched_at  = ?,
			plex_rating      = ?,
			in_plex          = 1
		WHERE id = (
			SELECT id FROM watch_events
			WHERE plex_activity_id IS NULL
			  AND lb_slug IS NOT NULL
			  AND LOWER(title) = LOWER(?)
			  AND lb_watched_on = substr(?, 1, 10)
			LIMIT 1
		)
	`)
	if err != nil {
		return err
	}
	defer mergeStmt.Close()

	// Fall-back: normal upsert keyed on plex_activity_id.
	upsertStmt, err := tx.Prepare(`
		INSERT INTO watch_events
			(title, year, plex_rating_key, plex_activity_id, plex_watched_at, plex_rating, in_plex)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(plex_activity_id) DO UPDATE SET
			title           = excluded.title,
			year            = excluded.year,
			plex_rating_key = excluded.plex_rating_key,
			plex_watched_at = excluded.plex_watched_at,
			plex_rating     = excluded.plex_rating,
			in_plex         = 1
	`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	for _, e := range entries {
		var activityID *string
		if e.ActivityID != "" {
			activityID = &e.ActivityID
		}

		res, err := mergeStmt.Exec(e.RatingKey, activityID, e.WatchedAt, e.Rating, e.Title, e.WatchedAt)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			if _, err := upsertStmt.Exec(e.Title, e.Year, e.RatingKey, activityID, e.WatchedAt, e.Rating); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Auto-detect rewatches: if multiple Plex rows share the same title,
	// all except the earliest are marked as rewatch.
	if _, err := db.Exec(`
		UPDATE watch_events SET plex_rewatch = 1
		WHERE in_plex = 1
		  AND plex_rewatch = 0
		  AND EXISTS (
			SELECT 1 FROM watch_events w2
			WHERE w2.in_plex = 1
			  AND LOWER(w2.title) = LOWER(watch_events.title)
			  AND w2.id != watch_events.id
			  AND COALESCE(substr(w2.plex_watched_at, 1, 10), '') < COALESCE(substr(watch_events.plex_watched_at, 1, 10), '')
		  )
	`); err != nil {
		log.Printf("[db] auto-detect plex rewatch error: %v", err)
	}

	return recordFetch(db, "plex")
}

// UpsertDiaryEntries writes Letterboxd diary entries into the DB.
// Keyed on (lb_slug, lb_watched_on) — one row per distinct watch date.
// If a matching Plex-only row exists (same title + same date, no LB data yet),
// it is merged in-place instead of creating a duplicate row.
func UpsertDiaryEntries(db *sql.DB, entries []DiaryEntryInput) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Try to merge into an existing Plex-only row with same title and watch date.
	mergeStmt, err := tx.Prepare(`
		UPDATE watch_events SET
			lb_slug       = ?,
			lb_watched_on = ?,
			lb_rating     = ?,
			lb_rewatch    = ?,
			in_lb         = 1
		WHERE id = (
			SELECT id FROM watch_events
			WHERE lb_slug IS NULL
			  AND plex_activity_id IS NOT NULL
			  AND LOWER(title) = LOWER(?)
			  AND substr(plex_watched_at, 1, 10) = ?
			LIMIT 1
		)
	`)
	if err != nil {
		return err
	}
	defer mergeStmt.Close()

	// Fall-back: normal upsert keyed on (lb_slug, lb_watched_on).
	upsertStmt, err := tx.Prepare(`
		INSERT INTO watch_events
			(title, year, lb_slug, lb_watched_on, lb_rating, lb_rewatch, in_lb)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(lb_slug, lb_watched_on) DO UPDATE SET
			title        = excluded.title,
			year         = excluded.year,
			lb_rating    = excluded.lb_rating,
			lb_rewatch   = excluded.lb_rewatch,
			in_lb        = 1
	`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	for _, e := range entries {
		rewatch := 0
		if e.Rewatch {
			rewatch = 1
		}

		res, err := mergeStmt.Exec(e.Slug, e.WatchedOn, e.Rating, rewatch, e.Title, e.WatchedOn)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			if _, err := upsertStmt.Exec(e.Title, e.Year, e.Slug, e.WatchedOn, e.Rating, rewatch); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return recordFetch(db, "letterboxd")
}

func recordFetch(db *sql.DB, source string) error {
	_, err := db.Exec(`
		INSERT INTO sync_log (source, last_fetched_at) VALUES (?, ?)
		ON CONFLICT(source) DO UPDATE SET last_fetched_at = excluded.last_fetched_at
	`, source, time.Now().UTC().Format(time.RFC3339))
	return err
}

// -------------------------------------------------------------------
// Query helpers
// -------------------------------------------------------------------

// AllWatchEvents returns all rows from the DB ordered by most-recently watched.
// Display values follow the LB-trumps-Plex rule.
func AllWatchEvents(db *sql.DB) ([]WatchEvent, error) {
	rows, err := db.Query(`
		SELECT
			id, title, year,
			plex_rating_key, plex_activity_id, plex_watched_at, plex_rating, plex_rewatch,
			lb_slug, lb_watched_on, lb_rating, lb_rewatch,
			in_plex, in_lb,
			plex_sync_status, lb_sync_status
		FROM watch_events
		ORDER BY COALESCE(lb_watched_on, substr(plex_watched_at, 1, 10)) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WatchEvent
	for rows.Next() {
		var e WatchEvent
		var plexRatingKey, plexActivityID, plexWatchedAt *string
		var plexRating *float64
		var plexRewatch int
		var lbSlug, lbWatchedOn *string
		var lbRating *float64
		var lbRewatch int
		var inPlex, inLb int
		var plexSyncStatus, lbSyncStatus *string

		if err := rows.Scan(
			&e.ID, &e.Title, &e.Year,
			&plexRatingKey, &plexActivityID, &plexWatchedAt, &plexRating, &plexRewatch,
			&lbSlug, &lbWatchedOn, &lbRating, &lbRewatch,
			&inPlex, &inLb,
			&plexSyncStatus, &lbSyncStatus,
		); err != nil {
			return nil, err
		}

		if plexRatingKey != nil {
			e.PlexRatingKey = *plexRatingKey
		}
		if plexActivityID != nil {
			e.PlexActivityID = *plexActivityID
		}
		if plexWatchedAt != nil {
			e.PlexWatchedAt = *plexWatchedAt
		}
		if plexRating != nil {
			e.PlexRating = *plexRating
		}
		if lbSlug != nil {
			e.LbSlug = *lbSlug
		}
		if lbWatchedOn != nil {
			e.LbWatchedOn = *lbWatchedOn
		}
		if lbRating != nil {
			e.LbRating = *lbRating
		}
		e.LbRewatch = lbRewatch == 1
		e.InPlex = inPlex == 1
		e.InLb = inLb == 1
		if plexSyncStatus != nil {
			e.PlexSyncStatus = *plexSyncStatus
		}
		if lbSyncStatus != nil {
			e.LbSyncStatus = *lbSyncStatus
		}

		// LB trumps Plex for display values
		if e.LbWatchedOn != "" {
			e.WatchedOn = e.LbWatchedOn
		} else if e.PlexWatchedAt != "" {
			e.WatchedOn = e.PlexWatchedAt[:10]
		}

		if e.LbRating != 0 {
			e.Rating = e.LbRating
		} else {
			e.Rating = e.PlexRating
		}

		e.Rewatch = e.LbRewatch || plexRewatch == 1

		events = append(events, e)
	}

	if events == nil {
		events = []WatchEvent{}
	}
	return events, rows.Err()
}

// ClearAll deletes all rows from watch_events and sync_log.
func ClearAll(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM watch_events`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM sync_log`)
	return err
}

// LastFetchedAt returns the timestamp when the given source was last synced.
// Returns empty string if never synced.
func LastFetchedAt(db *sql.DB, source string) string {
	var t string
	row := db.QueryRow(`SELECT last_fetched_at FROM sync_log WHERE source = ?`, source)
	if err := row.Scan(&t); err != nil {
		return ""
	}
	return t
}

// UpdatePlexWatchDate updates the plex_watched_at field for a row identified
// by its plex_activity_id. Returns the number of rows affected.
func UpdatePlexWatchDate(db *sql.DB, activityID, newDate string) (int64, error) {
	res, err := db.Exec(`UPDATE watch_events SET plex_watched_at = ? WHERE plex_activity_id = ?`, newDate, activityID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
