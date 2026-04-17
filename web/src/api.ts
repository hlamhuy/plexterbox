export interface WatchEvent {
    id: number;
    title: string;
    year: number;
    watchedOn: string;
    rating: number;
    rewatch: boolean;
    plexRatingKey?: string;
    plexActivityId?: string;
    plexWatchedAt?: string;
    plexRating?: number;
    lbSlug?: string;
    lbWatchedOn?: string;
    lbRating?: number;
    lbRewatch?: boolean;
    inPlex: boolean;
    inLb: boolean;
    plexSyncStatus?: string;
    lbSyncStatus?: string;
}

export async function fetchMovies(): Promise<{
    events: WatchEvent[];
    plexFetchedAt: string;
    lbFetchedAt: string;
}> {
    const res = await fetch('/api/movies');
    if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Unknown error' }));
        throw new Error(err.error || `HTTP ${res.status}`);
    }
    return res.json();
}

export async function syncData(): Promise<{
    events: WatchEvent[];
    plexFetchedAt: string;
    lbFetchedAt: string;
    errors: string[];
}> {
    const res = await fetch('/api/sync', { method: 'POST' });
    if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Unknown error' }));
        throw new Error(err.error || `HTTP ${res.status}`);
    }
    return res.json();
}

export async function letterboxdLogin(
    username: string,
    password: string,
): Promise<{ status: string; username?: string }> {
    const res = await fetch('/api/letterboxd/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
    });

    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }

    return data;
}

export async function letterboxdTOTP(
    code: string,
): Promise<{ status: string }> {
    const res = await fetch('/api/letterboxd/totp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
    });

    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }

    return data;
}

export async function letterboxdStatus(): Promise<{
    connected: boolean;
    username?: string;
}> {
    const res = await fetch('/api/letterboxd/status');
    return res.json();
}

export async function plexStatus(): Promise<{
    connected: boolean;
    username?: string;
}> {
    const res = await fetch('/api/plex/status');
    return res.json();
}

export async function plexLogout(): Promise<void> {
    await fetch('/api/plex/logout', { method: 'POST' });
}

export async function letterboxdLogout(): Promise<void> {
    await fetch('/api/letterboxd/logout', { method: 'POST' });
}

export async function deleteDatabase(): Promise<void> {
    const res = await fetch('/api/db', { method: 'DELETE' });
    if (!res.ok) throw new Error('Failed to delete database');
}

export async function plexOAuthStart(): Promise<{ authUrl: string }> {
    const res = await fetch('/api/plex/oauth/start', { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
}

export async function plexOAuthCheck(): Promise<{
    status: string;
    username?: string;
}> {
    const res = await fetch('/api/plex/oauth/check');
    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
}

export interface ImportFilm {
    title: string;
    originalTitle: string;
    rating: number;
    review: string | null;
    year: number;
    imdbId: string | null;
    letterboxdURI: string | null;
    tmdbId: string | null;
    tags: string | null;
    watchedDate: string;
    isICheckMoviesImport: boolean;
    rewatch: boolean;
    creators: unknown[];
}

export interface ImportResponse {
    status: string;
    filmStatuses: string[];
    imported: number;
    skipped: number;
    total: number;
    error?: string;
}

export async function letterboxdImport(
    films: ImportFilm[],
): Promise<ImportResponse> {
    const res = await fetch('/api/letterboxd/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ films }),
    });

    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
}

export interface PlexImportFilm {
    title: string;
    year: number;
    rating: number; // 1-10 scale
    watchedDate: string; // YYYY-MM-DD
}

export interface PlexImportResponse {
    status: string;
    filmStatuses: string[];
    imported: number;
    skipped: number;
    total: number;
}

export async function plexImport(
    films: PlexImportFilm[],
): Promise<PlexImportResponse> {
    const res = await fetch('/api/plex/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ films }),
    });

    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
}

export async function editPlexWatchDate(
    activityId: string,
    date: string,
): Promise<void> {
    const res = await fetch('/api/plex/activity/date', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ activityId, date }),
    });

    if (!res.ok) {
        const data = await res.json().catch(() => ({ error: 'Unknown error' }));
        throw new Error(data.error || `HTTP ${res.status}`);
    }
}

export async function setAutoSync(config: {
    mode: string;
    interval: string;
    direction: string;
}): Promise<void> {
    await fetch('/api/autosync', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config),
    });
}

export async function getAutoSync(): Promise<{
    mode: string;
    interval: string;
    direction: string;
    lastSyncAt: string;
}> {
    const res = await fetch('/api/autosync');
    return res.json();
}
