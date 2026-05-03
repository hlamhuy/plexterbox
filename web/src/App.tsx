import { useState, useEffect } from 'react';
import type { ImportFilm, PlexImportFilm, WatchEvent } from './api';
import {
    letterboxdImport,
    plexImport,
    fetchMovies,
    syncData,
    letterboxdStatus,
    plexStatus,
    plexLogout,
    letterboxdLogout,
    deleteDatabase,
} from './api';
import { formatTs } from './utils';
import PlexConfig from './components/PlexConfig';
import LetterboxdPanel from './components/LetterboxdPanel';
import AutoSyncPanel from './components/AutoSyncPanel';
import WatchTable from './components/WatchTable';
import LoadingModal from './components/modals/LoadingModal';
import SyncModal from './components/modals/SyncModal';
import DeleteConfirmModal from './components/modals/DeleteConfirmModal';

function App() {
    const [loadingMessage, setLoadingMessage] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [lbUser, setLbUser] = useState<string | null>(null);
    const [plexConnected, setPlexConnected] = useState(false);

    // Unified DB state
    const [watchEvents, setWatchEvents] = useState<WatchEvent[]>([]);
    const [lastFetched, setLastFetched] = useState<string | null>(null);

    // Import state
    const [importStatuses, setImportStatuses] = useState<Record<
        number,
        string
    > | null>(null);
    const [importResult, setImportResult] = useState<{
        imported: number;
        skipped: number;
        total: number;
    } | null>(null);

    // Modal state
    const [showSyncModal, setShowSyncModal] = useState(false);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

    // Load session status and populate table from DB on mount.
    useEffect(() => {
        plexStatus()
            .then((s) => {
                if (s.connected) setPlexConnected(true);
            })
            .catch(() => {});

        letterboxdStatus()
            .then((s) => {
                if (s.connected && s.username) setLbUser(s.username);
            })
            .catch(() => {});

        fetchMovies()
            .then(({ events, plexFetchedAt: pf, lbFetchedAt: lf }) => {
                setWatchEvents(events);
                const latest = pf && lf ? (pf > lf ? pf : lf) : pf || lf;
                if (latest) setLastFetched(formatTs(latest));
            })
            .catch(() => {});
    }, []);

    const handleFetchData = async () => {
        if (!plexConnected || !lbUser) return;
        setLoadingMessage('Fetching data from Plex and Letterboxd...');
        setError(null);
        try {
            const {
                events,
                plexFetchedAt: pf,
                lbFetchedAt: lf,
                errors,
            } = await syncData();
            setWatchEvents(events);
            const latest = pf && lf ? (pf > lf ? pf : lf) : pf || lf;
            if (latest) setLastFetched(formatTs(latest));
            if (errors && errors.length > 0) {
                setError('Fetch warnings: ' + errors.join('; '));
            }
        } catch (err) {
            setError(
                err instanceof Error ? err.message : 'Failed to fetch data',
            );
        } finally {
            setLoadingMessage(null);
        }
    };

    const handleDeleteDB = async () => {
        setShowDeleteConfirm(false);
        setLoadingMessage('Deleting database...');
        try {
            await deleteDatabase();
            setWatchEvents([]);
            setLastFetched(null);
            setImportStatuses(null);
            setImportResult(null);
        } catch (err) {
            setError(
                err instanceof Error
                    ? err.message
                    : 'Failed to delete database',
            );
        } finally {
            setLoadingMessage(null);
        }
    };

    const handlePlexToLb = async () => {
        setShowSyncModal(false);
        const plexEvents = watchEvents.filter((e) => e.inPlex && !e.inLb);
        if (plexEvents.length === 0 || !lbUser) return;
        setLoadingMessage('Syncing to Letterboxd...');
        setError(null);
        setImportResult(null);
        setImportStatuses(null);
        try {
            const films: ImportFilm[] = plexEvents.map((e) => ({
                title: e.title,
                originalTitle: e.title,
                rating: e.plexRating || 0,
                review: null,
                year: e.year,
                imdbId: null,
                letterboxdURI: null,
                tmdbId: null,
                tags: null,
                watchedDate: (e.plexWatchedAt || e.watchedOn || '').slice(
                    0,
                    10,
                ),
                isICheckMoviesImport: false,
                rewatch: e.rewatch,
                creators: [],
            }));
            const result = await letterboxdImport(films);
            const statusMap: Record<number, string> = {};
            plexEvents.forEach((e, idx) => {
                if (result.filmStatuses[idx]) {
                    statusMap[e.id] = result.filmStatuses[idx];
                }
            });
            setImportStatuses(statusMap);
            setImportResult({
                imported: result.imported,
                skipped: result.skipped,
                total: result.total,
            });

            // Refetch data so the table reflects new LB entries
            setLoadingMessage('Refreshing data...');
            const {
                events: refreshed,
                plexFetchedAt: pf,
                lbFetchedAt: lf,
            } = await syncData();
            setWatchEvents(refreshed);
            const latest = pf && lf ? (pf > lf ? pf : lf) : pf || lf;
            if (latest) setLastFetched(formatTs(latest));
        } catch (err) {
            setError(
                err instanceof Error
                    ? err.message
                    : 'Failed to import to Letterboxd',
            );
        } finally {
            setLoadingMessage(null);
        }
    };

    const handleLbToPlex = async () => {
        setShowSyncModal(false);
        const lbEvents = watchEvents.filter((e) => e.inLb && !e.inPlex);
        if (lbEvents.length === 0 || !plexConnected) return;
        setLoadingMessage('Syncing to Plex...');
        setError(null);
        setImportResult(null);
        setImportStatuses(null);
        try {
            const films: PlexImportFilm[] = lbEvents.map((e) => ({
                title: e.title,
                year: e.year,
                rating: e.lbRating || 0,
                watchedDate: e.lbWatchedOn || e.watchedOn || '',
            }));
            const result = await plexImport(films);
            const statusMap: Record<number, string> = {};
            lbEvents.forEach((e, idx) => {
                if (result.filmStatuses[idx]) {
                    statusMap[e.id] = result.filmStatuses[idx];
                }
            });
            setImportStatuses(statusMap);
            setImportResult({
                imported: result.imported,
                skipped: result.skipped,
                total: result.total,
            });

            // Refetch data so the table reflects new Plex entries
            setLoadingMessage('Refreshing data...');
            const {
                events: refreshed,
                plexFetchedAt: pf,
                lbFetchedAt: lf,
            } = await syncData();
            setWatchEvents(refreshed);
            const latest = pf && lf ? (pf > lf ? pf : lf) : pf || lf;
            if (latest) setLastFetched(formatTs(latest));
        } catch (err) {
            setError(
                err instanceof Error ? err.message : 'Failed to import to Plex',
            );
        } finally {
            setLoadingMessage(null);
        }
    };

    const handleFullSync = async () => {
        setShowSyncModal(false);
        const plexEvents = watchEvents.filter((e) => e.inPlex && !e.inLb);
        const lbEvents = watchEvents.filter((e) => e.inLb && !e.inPlex);
        if (
            (plexEvents.length === 0 || !lbUser) &&
            (lbEvents.length === 0 || !plexConnected)
        )
            return;

        setError(null);
        setImportResult(null);
        setImportStatuses(null);

        let totalImported = 0;
        let totalSkipped = 0;
        let totalTotal = 0;
        const combinedStatuses: Record<number, string> = {};

        // Plex → Letterboxd
        if (plexEvents.length > 0 && lbUser) {
            setLoadingMessage('Syncing Plex → Letterboxd...');
            try {
                const films: ImportFilm[] = plexEvents.map((e) => ({
                    title: e.title,
                    originalTitle: e.title,
                    rating: e.plexRating || 0,
                    review: null,
                    year: e.year,
                    imdbId: null,
                    letterboxdURI: null,
                    tmdbId: null,
                    tags: null,
                    watchedDate: (e.plexWatchedAt || e.watchedOn || '').slice(
                        0,
                        10,
                    ),
                    isICheckMoviesImport: false,
                    rewatch: e.rewatch,
                    creators: [],
                }));
                const result = await letterboxdImport(films);
                plexEvents.forEach((e, idx) => {
                    if (result.filmStatuses[idx])
                        combinedStatuses[e.id] = result.filmStatuses[idx];
                });
                totalImported += result.imported;
                totalSkipped += result.skipped;
                totalTotal += result.total;
            } catch (err) {
                setError(
                    err instanceof Error
                        ? err.message
                        : 'Failed to import to Letterboxd',
                );
            }
        }

        // Letterboxd → Plex
        if (lbEvents.length > 0 && plexConnected) {
            setLoadingMessage('Syncing Letterboxd → Plex...');
            try {
                const films: PlexImportFilm[] = lbEvents.map((e) => ({
                    title: e.title,
                    year: e.year,
                    rating: e.lbRating || 0,
                    watchedDate: e.lbWatchedOn || e.watchedOn || '',
                }));
                const result = await plexImport(films);
                lbEvents.forEach((e, idx) => {
                    if (result.filmStatuses[idx])
                        combinedStatuses[e.id] = result.filmStatuses[idx];
                });
                totalImported += result.imported;
                totalSkipped += result.skipped;
                totalTotal += result.total;
            } catch (err) {
                setError(
                    err instanceof Error
                        ? err.message
                        : 'Failed to import to Plex',
                );
            }
        }

        if (Object.keys(combinedStatuses).length > 0)
            setImportStatuses(combinedStatuses);
        if (totalTotal > 0)
            setImportResult({
                imported: totalImported,
                skipped: totalSkipped,
                total: totalTotal,
            });

        // Single refresh at the end
        setLoadingMessage('Refreshing data...');
        try {
            const {
                events: refreshed,
                plexFetchedAt: pf,
                lbFetchedAt: lf,
            } = await syncData();
            setWatchEvents(refreshed);
            const latest = pf && lf ? (pf > lf ? pf : lf) : pf || lf;
            if (latest) setLastFetched(formatTs(latest));
        } catch {}
        setLoadingMessage(null);
    };

    return (
        <div className='min-h-screen bg-zinc-950 text-zinc-100'>
            <div className='max-w-5xl mx-auto px-4 py-8'>
                <h1 className='text-3xl font-bold mb-2'>Plexterbox</h1>
                <p className='text-zinc-400 mb-8'>
                    Sync your watch history between Plex and Letterboxd
                </p>

                <div className='grid grid-cols-2 gap-4'>
                    <PlexConfig
                        onLinked={() => setPlexConnected(true)}
                        onDisconnect={async () => {
                            await plexLogout();
                            setPlexConnected(false);
                        }}
                        initialLinked={plexConnected}
                    />

                    <LetterboxdPanel
                        onLogin={setLbUser}
                        onDisconnect={async () => {
                            await letterboxdLogout();
                            setLbUser(null);
                        }}
                        initialUsername={lbUser}
                    />
                </div>

                <AutoSyncPanel
                    plexConnected={plexConnected}
                    lbUser={lbUser}
                    onSyncComplete={setWatchEvents}
                />

                <button
                    onClick={handleFetchData}
                    disabled={!!loadingMessage || !plexConnected || !lbUser}
                    className='mt-4 w-full px-4 py-3 bg-blue-700 hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg text-sm font-medium transition-colors cursor-pointer'
                >
                    Fetch Data
                </button>

                {error && (
                    <div className='mt-4 p-3 bg-red-900/50 border border-red-700 rounded text-red-200 text-sm'>
                        {error}
                    </div>
                )}

                {/* Unified watch table */}
                <div className='mt-8 bg-zinc-900 rounded-lg p-6 border border-zinc-800'>
                    <div className='flex items-start justify-between mb-4'>
                        <div>
                            <h2 className='text-xl font-semibold'>
                                Watch History ({watchEvents.length})
                            </h2>
                            {lastFetched && (
                                <p className='text-xs text-zinc-500 mt-0.5'>
                                    Last fetched: {lastFetched}
                                </p>
                            )}
                        </div>
                        <div className='flex gap-2 flex-shrink-0'>
                            {watchEvents.length > 0 && (
                                <button
                                    onClick={() => {
                                        setImportStatuses(null);
                                        setImportResult(null);
                                        fetchMovies()
                                            .then(({ events }) =>
                                                setWatchEvents(events),
                                            )
                                            .catch(() => {});
                                    }}
                                    className='p-2 rounded border border-zinc-700 text-zinc-400 hover:bg-zinc-700/30 transition-colors cursor-pointer'
                                    title='Refresh table'
                                >
                                    <svg
                                        xmlns='http://www.w3.org/2000/svg'
                                        viewBox='0 0 20 20'
                                        fill='currentColor'
                                        className='w-4 h-4'
                                    >
                                        <path
                                            fillRule='evenodd'
                                            d='M15.312 11.424a5.5 5.5 0 0 1-9.201 2.466l-.312-.311h2.433a.75.75 0 0 0 0-1.5H4.598a.75.75 0 0 0-.75.75v3.634a.75.75 0 0 0 1.5 0v-2.033l.312.311a7 7 0 0 0 11.712-3.138.75.75 0 0 0-1.449-.39Zm-10.624-3.85a5.5 5.5 0 0 1 9.201-2.465l.312.31H11.77a.75.75 0 0 0 0 1.5h3.634a.75.75 0 0 0 .75-.75V2.535a.75.75 0 0 0-1.5 0v2.033l-.312-.31A7 7 0 0 0 2.63 7.392a.75.75 0 0 0 1.45.39l.007-.02Z'
                                            clipRule='evenodd'
                                        />
                                    </svg>
                                </button>
                            )}
                            {watchEvents.length > 0 && (
                                <button
                                    onClick={() => setShowDeleteConfirm(true)}
                                    className='p-2 rounded border border-red-700 text-red-400 hover:bg-red-700/20 transition-colors cursor-pointer'
                                    title='Delete database'
                                >
                                    <svg
                                        xmlns='http://www.w3.org/2000/svg'
                                        viewBox='0 0 20 20'
                                        fill='currentColor'
                                        className='w-4 h-4'
                                    >
                                        <path
                                            fillRule='evenodd'
                                            d='M8.75 1A2.75 2.75 0 0 0 6 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 1 0 .23 1.482l.149-.022.841 10.518A2.75 2.75 0 0 0 7.596 19h4.807a2.75 2.75 0 0 0 2.742-2.53l.841-10.52.149.023a.75.75 0 0 0 .23-1.482A41.03 41.03 0 0 0 14 4.193V3.75A2.75 2.75 0 0 0 11.25 1h-2.5ZM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4ZM8.58 7.72a.75.75 0 0 0-1.5.06l.3 7.5a.75.75 0 1 0 1.5-.06l-.3-7.5Zm4.34.06a.75.75 0 1 0-1.5-.06l-.3 7.5a.75.75 0 1 0 1.5.06l.3-7.5Z'
                                            clipRule='evenodd'
                                        />
                                    </svg>
                                </button>
                            )}
                            <button
                                onClick={() => setShowSyncModal(true)}
                                disabled={
                                    !!loadingMessage || watchEvents.length === 0
                                }
                                className='px-12 py-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium transition-colors cursor-pointer'
                            >
                                Sync
                            </button>
                        </div>
                    </div>

                    {importResult && (
                        <div className='mb-4 grid grid-cols-3 gap-3 text-sm'>
                            <div className='p-3 bg-green-900/50 border border-green-700 rounded text-green-200 text-center'>
                                <div className='text-2xl font-bold'>
                                    {importResult.imported}
                                </div>
                                <div>Imported</div>
                            </div>
                            <div className='p-3 bg-yellow-900/50 border border-yellow-700 rounded text-yellow-200 text-center'>
                                <div className='text-2xl font-bold'>
                                    {importResult.skipped}
                                </div>
                                <div>Skipped</div>
                            </div>
                            <div className='p-3 bg-red-900/50 border border-red-700 rounded text-red-200 text-center'>
                                <div className='text-2xl font-bold'>
                                    {importResult.total -
                                        importResult.imported -
                                        importResult.skipped}
                                </div>
                                <div>Not Found</div>
                            </div>
                        </div>
                    )}

                    <WatchTable
                        events={watchEvents}
                        importStatuses={importStatuses}
                        onDateEdited={() => {
                            fetchMovies()
                                .then(({ events }) => setWatchEvents(events))
                                .catch(() => {});
                        }}
                    />
                </div>

                {/* Sync Modal */}
                {showSyncModal && (
                    <SyncModal
                        hasPlexOnly={watchEvents.some(
                            (e) => e.inPlex && !e.inLb,
                        )}
                        hasLbOnly={watchEvents.some((e) => e.inLb && !e.inPlex)}
                        hasLbUser={!!lbUser}
                        onFullSync={handleFullSync}
                        onPlexToLb={handlePlexToLb}
                        onLbToPlex={handleLbToPlex}
                        onClose={() => setShowSyncModal(false)}
                    />
                )}

                {/* Delete Confirmation Modal */}
                {showDeleteConfirm && (
                    <DeleteConfirmModal
                        onConfirm={handleDeleteDB}
                        onCancel={() => setShowDeleteConfirm(false)}
                    />
                )}

                {/* Loading Modal */}
                {loadingMessage && <LoadingModal message={loadingMessage} />}
            </div>
        </div>
    );
}

export default App;
