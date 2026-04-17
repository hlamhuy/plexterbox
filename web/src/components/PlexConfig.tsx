import { useState, useEffect, useRef } from 'react';
import { plexOAuthStart, plexOAuthCheck } from '../api';

interface PlexConfigProps {
    onLinked: () => void;
    onDisconnect?: () => void;
    initialLinked?: boolean;
}

export default function PlexConfig({
    onLinked,
    onDisconnect,
    initialLinked,
}: PlexConfigProps) {
    const [linked, setLinked] = useState(initialLinked ?? false);

    useEffect(() => {
        if (initialLinked) setLinked(true);
    }, [initialLinked]);

    const [oauthLoading, setOauthLoading] = useState(false);
    const [oauthError, setOauthError] = useState<string | null>(null);
    const pollRef = useRef<number | null>(null);

    const stopPolling = () => {
        if (pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
        }
    };

    useEffect(() => () => stopPolling(), []);

    const handleOAuth = async () => {
        setOauthLoading(true);
        setOauthError(null);
        try {
            const { authUrl } = await plexOAuthStart();
            const popup = window.open(
                authUrl,
                'plex-auth',
                'width=800,height=600',
            );

            pollRef.current = window.setInterval(async () => {
                try {
                    const result = await plexOAuthCheck();
                    if (result.status === 'ok' && result.username) {
                        stopPolling();
                        popup?.close();
                        setOauthLoading(false);
                        setLinked(true);
                        onLinked();
                    }
                } catch {
                    stopPolling();
                    setOauthError('Failed to check authentication');
                    setOauthLoading(false);
                }
            }, 2000);
        } catch (err) {
            setOauthError(
                err instanceof Error ? err.message : 'Failed to start OAuth',
            );
            setOauthLoading(false);
        }
    };

    const handleDisconnect = () => {
        setLinked(false);
        onDisconnect?.();
    };

    return (
        <div className='bg-zinc-900 p-6 rounded-lg border border-zinc-800'>
            <h2 className='text-lg font-semibold mb-4'>Plex</h2>

            {linked ? (
                <div className='space-y-3'>
                    <div className='flex items-center justify-between'>
                        <span className='text-sm text-green-400'>
                            Plex account linked
                        </span>
                        <button
                            onClick={handleDisconnect}
                            className='text-xs text-zinc-500 hover:text-zinc-300 cursor-pointer'
                        >
                            Disconnect
                        </button>
                    </div>
                </div>
            ) : (
                <div className='space-y-3'>
                    <button
                        onClick={handleOAuth}
                        disabled={oauthLoading}
                        className='w-full px-4 py-2 bg-orange-600 hover:bg-orange-700 disabled:opacity-50 rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        {oauthLoading
                            ? 'Waiting for Plex login...'
                            : 'Link Plex Account'}
                    </button>
                    {oauthError && (
                        <p className='text-red-400 text-sm'>{oauthError}</p>
                    )}
                </div>
            )}
        </div>
    );
}
