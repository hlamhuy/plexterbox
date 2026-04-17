import { useState, useEffect } from 'react';
import LetterboxdLogin from './LetterboxdLogin';

interface LetterboxdPanelProps {
    onLogin: (username: string) => void;
    onDisconnect: () => void;
    initialUsername?: string | null;
}

export default function LetterboxdPanel({
    onLogin,
    onDisconnect,
    initialUsername,
}: LetterboxdPanelProps) {
    const [username, setUsername] = useState<string | null>(
        initialUsername ?? null,
    );

    // Sync when the parent resolves the session asynchronously on mount
    useEffect(() => {
        if (initialUsername) setUsername(initialUsername);
    }, [initialUsername]);

    const handleLogin = (u: string) => {
        setUsername(u);
        onLogin(u);
    };

    const handleDisconnect = () => {
        setUsername(null);
        onDisconnect();
    };

    if (!username) {
        return <LetterboxdLogin onLogin={handleLogin} />;
    }

    return (
        <div className='bg-zinc-900 rounded-lg p-6 border border-zinc-800'>
            <h2 className='text-lg font-semibold mb-4'>Letterboxd</h2>
            <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                    <span className='text-sm text-green-400'>
                        Signed in as <strong>{username}</strong>
                    </span>
                    <button
                        onClick={handleDisconnect}
                        className='text-xs text-zinc-500 hover:text-zinc-300 cursor-pointer'
                    >
                        Disconnect
                    </button>
                </div>
            </div>
        </div>
    );
}
