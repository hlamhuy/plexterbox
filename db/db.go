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
	ViewingID string  // data-viewing-id — stable diary entry ID
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
			lb_viewing_id    TEXT,

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
			last_fetched_at TEXT NOT NULL,
			last_seen_id    TEXT
		);
	`)
	if err != nil {
		return err
	}

	// Add plex_rewatch column for existing databases.
	_, _ = db.Exec(`ALTER TABLE watch_events ADD COLUMN plex_rewatch INTEGER NOT NULL DEFAULT 0`)

	// Add lb_viewing_id for existing databases.
	_, _ = db.Exec(`ALTER TABLE watch_events ADD COLUMN lb_viewing_id TEXT`)
	// Full unique index (not partial) — required for ON CONFLICT(lb_viewing_id) upsert syntax.
	// SQLite treats NULLs as distinct, so multiple NULL values are allowed.
	_, _ = db.Exec(`DROP INDEX IF EXISTS idx_lb_viewing_id`)
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_lb_viewing_id ON watch_events(lb_viewing_id)`)

	// Add last_seen_id to sync_log for existing databases.
	_, _ = db.Exec(`ALTER TABLE sync_log ADD COLUMN last_seen_id TEXT`)

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

	lastSeenID := ""
	if len(entries) > 0 {
		lastSeenID = entries[0].ActivityID
	}
	return recordFetch(db, "plex", lastSeenID)
}

// UpsertDiaryEntries writes Letterboxd diary entries into the DB.
// When an entry has a ViewingID it is keyed on lb_viewing_id (stable), so a
// date change on Letterboxd updates the existing row instead of inserting a
// duplicate. Entries without a ViewingID fall back to (lb_slug, lb_watched_on).
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
			lb_viewing_id = ?,
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

	// Backfill lb_viewing_id on any existing LB row that matches by (slug, date)
	// but has no viewing ID yet. This prevents a UNIQUE(lb_slug, lb_watched_on)
	// conflict when we then upsert by lb_viewing_id below.
	backfillStmt, err := tx.Prepare(`
		UPDATE watch_events
		SET lb_viewing_id = ?
		WHERE lb_viewing_id IS NULL
		  AND lb_slug = ?
		  AND lb_watched_on = ?
	`)
	if err != nil {
		return err
	}
	defer backfillStmt.Close()

	// Upsert keyed on lb_viewing_id. Updates lb_watched_on/lb_slug so a date
	// change on Letterboxd is reflected without creating a duplicate row.
	upsertByViewingIDStmt, err := tx.Prepare(`
		INSERT INTO watch_events
			(title, year, lb_slug, lb_viewing_id, lb_watched_on, lb_rating, lb_rewatch, in_lb)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(lb_viewing_id) DO UPDATE SET
			title         = excluded.title,
			year          = excluded.year,
			lb_slug       = excluded.lb_slug,
			lb_watched_on = excluded.lb_watched_on,
			lb_rating     = excluded.lb_rating,
			lb_rewatch    = excluded.lb_rewatch,
			in_lb         = 1
	`)
	if err != nil {
		return err
	}
	defer upsertByViewingIDStmt.Close()

	// Fall-back: upsert keyed on (lb_slug, lb_watched_on) for entries without a viewing ID.
	upsertBySlugDateStmt, err := tx.Prepare(`
		INSERT INTO watch_events
			(title, year, lb_slug, lb_watched_on, lb_rating, lb_rewatch, in_lb)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(lb_slug, lb_watched_on) DO UPDATE SET
			title      = excluded.title,
			year       = excluded.year,
			lb_rating  = excluded.lb_rating,
			lb_rewatch = excluded.lb_rewatch,
			in_lb      = 1
	`)
	if err != nil {
		return err
	}
	defer upsertBySlugDateStmt.Close()

	for _, e := range entries {
		rewatch := 0
		if e.Rewatch {
			rewatch = 1
		}

		var viewingID *string
		if e.ViewingID != "" {
			viewingID = &e.ViewingID
		}

		// Step 1: try to merge into a Plex-only row with matching title + date.
		res, err := mergeStmt.Exec(e.Slug, viewingID, e.WatchedOn, e.Rating, rewatch, e.Title, e.WatchedOn)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			continue
		}

		// Step 2: upsert by viewing ID or (slug, date) fallback.
		if e.ViewingID != "" {
			// Backfill any existing row that matches slug+date so the upcoming
			// insert by lb_viewing_id doesn't trip the (lb_slug, lb_watched_on) constraint.
			if _, err := backfillStmt.Exec(e.ViewingID, e.Slug, e.WatchedOn); err != nil {
				return err
			}
			if _, err := upsertByViewingIDStmt.Exec(e.Title, e.Year, e.Slug, e.ViewingID, e.WatchedOn, e.Rating, rewatch); err != nil {
				return err
			}
		} else {
			if _, err := upsertBySlugDateStmt.Exec(e.Title, e.Year, e.Slug, e.WatchedOn, e.Rating, rewatch); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	lastSeenID := ""
	if len(entries) > 0 {
		lastSeenID = entries[0].ViewingID
	}
	return recordFetch(db, "letterboxd", lastSeenID)
}

func recordFetch(db *sql.DB, source, lastSeenID string) error {
	_, err := db.Exec(`
		INSERT INTO sync_log (source, last_fetched_at, last_seen_id) VALUES (?, ?, NULLIF(?, ''))
		ON CONFLICT(source) DO UPDATE SET
			last_fetched_at = excluded.last_fetched_at,
			last_seen_id    = CASE WHEN excluded.last_seen_id IS NOT NULL THEN excluded.last_seen_id ELSE sync_log.last_seen_id END
	`, source, time.Now().UTC().Format(time.RFC3339), lastSeenID)
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

// LastSeenID returns the most recently seen entry ID for the given source.
// Returns empty string if no sync has occurred or no ID was recorded.
// This is used by autosync to stop pagination once already-seen entries are reached.
func LastSeenID(db *sql.DB, source string) string {
	var id string
	row := db.QueryRow(`SELECT COALESCE(last_seen_id, '') FROM sync_log WHERE source = ?`, source)
	if err := row.Scan(&id); err != nil {
		return ""
	}
	return id
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
