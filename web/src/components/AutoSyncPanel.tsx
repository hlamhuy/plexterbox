import { useState, useEffect, useRef } from 'react';
import type { WatchEvent } from '../api';
import { setAutoSync, getAutoSync, fetchMovies } from '../api';
import { formatTs } from '../utils';

interface Props {
    plexConnected: boolean;
    lbUser: string | null;
    onSyncComplete: (events: WatchEvent[]) => void;
}

const INTERVAL_OPTIONS: [string, string][] = [
    ['5m', '5 min'],
    ['15m', '15 min'],
    ['30m', '30 min'],
    ['60m', '60 min'],
    ['3h', '3 hours'],
    ['6h', '6 hours'],
    ['12h', '12 hours'],
    ['24h', '24 hours'],
];

export default function AutoSyncPanel({
    plexConnected,
    lbUser,
    onSyncComplete,
}: Props) {
    const [mode, setMode] = useState<'disabled' | 'safe' | 'fast'>('disabled');
    const [syncInterval, setSyncInterval] = useState('15m');
    const [direction, setDirection] = useState<
        'full' | 'plexToLb' | 'lbToPlex'
    >('full');
    const [lastAt, setLastAt] = useState<string | null>(null);
    const [saved, setSaved] = useState(false);
    const [dirty, setDirty] = useState(false);
    const [showInfo, setShowInfo] = useState(false);
    const savedCfg = useRef({
        mode: 'disabled',
        interval: '15m',
        direction: 'full',
    });

    const disabled = !plexConnected || !lbUser;

    const markDirty = (m: string, i: string, d: string) => {
        const s = savedCfg.current;
        setDirty(m !== s.mode || i !== s.interval || d !== s.direction);
    };

    const handleSave = () => {
        setAutoSync({ mode, interval: syncInterval, direction })
            .then(() => {
                savedCfg.current = { mode, interval: syncInterval, direction };
                setDirty(false);
                setSaved(true);
                setTimeout(() => setSaved(false), 2000);
            })
            .catch(() => {});
    };

    // Load saved settings on mount.
    useEffect(() => {
        getAutoSync()
            .then((s) => {
                setMode(s.mode as 'disabled' | 'safe' | 'fast');
                setSyncInterval(s.interval ?? '15m');
                setDirection(
                    (s.direction ?? 'full') as 'full' | 'plexToLb' | 'lbToPlex',
                );
                if (s.lastSyncAt) setLastAt(s.lastSyncAt);
                savedCfg.current = {
                    mode: s.mode,
                    interval: s.interval ?? '15m',
                    direction: s.direction ?? 'full',
                };
            })
            .catch(() => {});
    }, []);

    // Poll every 30s in safe mode; refresh table when a job completes.
    useEffect(() => {
        if (mode !== 'safe') return;
        let prevLastAt: string | null = null;
        const id = window.setInterval(async () => {
            try {
                const status = await getAutoSync();
                if (status.lastSyncAt && status.lastSyncAt !== prevLastAt) {
                    prevLastAt = status.lastSyncAt;
                    setLastAt(status.lastSyncAt);
                    const { events } = await fetchMovies();
                    onSyncComplete(events);
                }
            } catch {}
        }, 30_000);
        return () => window.clearInterval(id);
    }, [mode, onSyncComplete]);

    const selectClass =
        'w-44 bg-zinc-900 border border-zinc-600 rounded px-2 py-1 text-sm text-zinc-200 cursor-pointer focus:outline-none focus:border-zinc-500 disabled:opacity-40 disabled:cursor-not-allowed';

    return (
        <div className='mt-4 bg-zinc-900 rounded-lg p-6 border border-zinc-800'>
            <div className='flex items-center justify-between mb-4'>
                <h2 className='text-lg font-semibold'>Auto Sync</h2>
                <div className='flex items-center gap-2'>
                    <button
                        onClick={() => setShowInfo(true)}
                        className='w-6 h-6 flex items-center justify-center rounded-full border border-zinc-600 text-zinc-400 text-xs hover:border-zinc-400 hover:text-zinc-200 transition-colors cursor-pointer'
                        title='About Auto Sync'
                    >
                        ?
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={!dirty}
                        className='w-28 px-3 py-1.5 rounded-md text-sm font-medium transition-colors border border-green-600 text-green-600 disabled:border-zinc-600 disabled:text-zinc-600 enabled:hover:bg-green-500 enabled:hover:border-green-500 enabled:hover:text-zinc-100 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer'
                    >
                        {saved ? 'Saved ✓' : 'Save'}
                    </button>
                    <select
                        value={mode}
                        disabled={disabled}
                        onChange={(e) => {
                            const v = e.target.value as
                                | 'disabled'
                                | 'safe'
                                | 'fast';
                            setMode(v);
                            markDirty(v, syncInterval, direction);
                        }}
                        className='w-28 bg-zinc-800 border border-zinc-700 rounded-md px-3 py-1.5 text-sm text-zinc-200 cursor-pointer focus:outline-none focus:border-zinc-500 disabled:opacity-40 disabled:cursor-not-allowed'
                    >
                        <option value='disabled'>Disabled</option>
                        <option value='safe'>Safe Mode</option>
                        <option value='fast'>Fast Mode</option>
                    </select>
                </div>
            </div>

            {showInfo && (
                <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/60'>
                    <div className='bg-zinc-900 border border-zinc-700 rounded-xl p-6 max-w-sm w-full mx-4 shadow-xl'>
                        <h3 className='text-base font-semibold mb-3'>
                            About Auto Sync
                        </h3>
                        <div className='space-y-2 text-sm text-zinc-400'>
                            <p>
                                Auto Sync keeps your Plex and Letterboxd watch
                                histories in sync automatically, without manual
                                imports.{' '}
                                <span className='text-zinc-200 font-bold'>
                                    You must connect to both platforms to use
                                    this feature.
                                </span>
                            </p>
                            <p>
                                <span className='text-zinc-100'>Safe Mode</span>{' '}
                                — polls for new entries on a configurable
                                interval. Syncs only when new entries are
                                detected, and only in the specified direction,
                                to minimize risk of duplicates or data loss.
                            </p>
                            <p>
                                <span className='text-zinc-100'>Fast Mode</span>{' '}
                                — coming soon.
                            </p>
                        </div>
                        <button
                            onClick={() => setShowInfo(false)}
                            className='mt-5 w-full py-1.5 rounded-md border border-zinc-600 text-sm text-zinc-300 hover:bg-zinc-800 transition-colors cursor-pointer'
                        >
                            Close
                        </button>
                    </div>
                </div>
            )}

            {lastAt && (
                <p className='text-xs text-zinc-500 mb-3'>
                    Last synced: {formatTs(lastAt)}
                </p>
            )}

            {mode === 'safe' && (
                <div className='space-y-3 p-4 bg-zinc-800/60 rounded-lg border border-zinc-700'>
                    <div className='flex items-center justify-between'>
                        <span className='text-sm'>Sync interval</span>
                        <select
                            value={syncInterval}
                            onChange={(e) => {
                                setSyncInterval(e.target.value);
                                markDirty(mode, e.target.value, direction);
                            }}
                            disabled={disabled}
                            className={selectClass}
                        >
                            {INTERVAL_OPTIONS.map(([v, label]) => (
                                <option key={v} value={v}>
                                    {label}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className='flex items-center justify-between'>
                        <span className='text-sm'>Sync direction</span>
                        <select
                            value={direction}
                            onChange={(e) => {
                                setDirection(
                                    e.target.value as
                                        | 'full'
                                        | 'plexToLb'
                                        | 'lbToPlex',
                                );
                                markDirty(mode, syncInterval, e.target.value);
                            }}
                            disabled={disabled}
                            className={selectClass}
                        >
                            <option value='full'>Full Sync</option>
                            <option value='plexToLb'>Plex → Letterboxd</option>
                            <option value='lbToPlex'>Letterboxd → Plex</option>
                        </select>
                    </div>
                </div>
            )}

            {mode === 'fast' && (
                <div className='p-4 rounded-lg border border-zinc-700 text-center'>
                    <p className='text-zinc-500 text-sm'>Coming soon</p>
                </div>
            )}
        </div>
    );
}
