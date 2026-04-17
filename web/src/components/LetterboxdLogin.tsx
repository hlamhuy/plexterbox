import { useState } from 'react';
import { letterboxdLogin, letterboxdTOTP } from '../api';

interface Props {
    onLogin: (username: string) => void;
}

export default function LetterboxdLogin({ onLogin }: Props) {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [totpCode, setTotpCode] = useState('');
    const [needsTOTP, setNeedsTOTP] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError(null);
        try {
            const data = await letterboxdLogin(username, password);
            if (data.status === 'totp_required') {
                setNeedsTOTP(true);
            } else {
                onLogin(data.username ?? username);
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Login failed');
        } finally {
            setLoading(false);
        }
    };

    const handleTOTP = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError(null);
        try {
            await letterboxdTOTP(totpCode);
            onLogin(username);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Invalid code');
        } finally {
            setLoading(false);
        }
    };

    if (needsTOTP) {
        return (
            <form
                onSubmit={handleTOTP}
                className='bg-zinc-900 rounded-lg p-6 border border-zinc-800'
            >
                <h2 className='text-lg font-semibold mb-2'>
                    Two-Factor Authentication
                </h2>
                <p className='text-zinc-400 text-sm mb-4'>
                    Enter the code from your authenticator app.
                </p>
                <input
                    type='text'
                    inputMode='numeric'
                    autoComplete='one-time-code'
                    placeholder='6-digit code'
                    value={totpCode}
                    onChange={(e) => setTotpCode(e.target.value)}
                    className='w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded text-sm focus:outline-none focus:border-zinc-500 tracking-widest text-center text-lg'
                    maxLength={6}
                    required
                    autoFocus
                />
                {error && <p className='mt-3 text-red-400 text-sm'>{error}</p>}
                <button
                    type='submit'
                    disabled={loading || totpCode.length < 6}
                    className='mt-4 w-full px-4 py-2 bg-orange-600 hover:bg-orange-700 disabled:opacity-50 rounded text-sm font-medium transition-colors cursor-pointer'
                >
                    {loading ? 'Verifying...' : 'Verify'}
                </button>
            </form>
        );
    }

    return (
        <form
            onSubmit={handleSubmit}
            className='bg-zinc-900 rounded-lg p-6 border border-zinc-800'
        >
            <h2 className='text-lg font-semibold mb-4'>Letterboxd Login</h2>
            <div className='space-y-3'>
                <input
                    type='text'
                    placeholder='Username or email'
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className='w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded text-sm focus:outline-none focus:border-zinc-500'
                    required
                />
                <input
                    type='password'
                    placeholder='Password'
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className='w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded text-sm focus:outline-none focus:border-zinc-500'
                    required
                />
            </div>
            {error && <p className='mt-3 text-red-400 text-sm'>{error}</p>}
            <button
                type='submit'
                disabled={loading}
                className='mt-4 w-full px-4 py-2 bg-orange-600 hover:bg-orange-700 disabled:opacity-50 rounded text-sm font-medium transition-colors cursor-pointer'
            >
                {loading ? 'Signing in...' : 'Sign in to Letterboxd'}
            </button>
        </form>
    );
}
