interface SyncModalProps {
    hasPlexOnly: boolean;
    hasLbOnly: boolean;
    hasLbUser: boolean;
    onFullSync: () => void;
    onPlexToLb: () => void;
    onLbToPlex: () => void;
    onClose: () => void;
}

export default function SyncModal({
    hasPlexOnly,
    hasLbOnly,
    hasLbUser,
    onFullSync,
    onPlexToLb,
    onLbToPlex,
    onClose,
}: SyncModalProps) {
    const plexToLbDisabled = !hasPlexOnly || !hasLbUser;
    const lbToPlexDisabled = !hasLbOnly;

    return (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/60'>
            <div className='bg-zinc-900 border border-zinc-700 rounded-lg p-8 w-120 shadow-xl'>
                <h3 className='text-lg font-semibold mb-4'>Sync Options</h3>
                <div className='space-y-4'>
                    <button
                        onClick={onFullSync}
                        disabled={plexToLbDisabled && lbToPlexDisabled}
                        className='w-full px-4 py-2.5 bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        Full Sync
                    </button>
                    <button
                        onClick={onPlexToLb}
                        disabled={plexToLbDisabled}
                        className='w-full px-4 py-2.5 bg-zinc-700 hover:bg-zinc-600 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        Plex → Letterboxd
                    </button>
                    <button
                        onClick={onLbToPlex}
                        disabled={lbToPlexDisabled}
                        className='w-full px-4 py-2.5 bg-zinc-700 hover:bg-zinc-600 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        Letterboxd → Plex
                    </button>
                </div>
                <button
                    onClick={onClose}
                    className='mt-4 w-full px-4 py-2 text-sm text-zinc-400 hover:text-zinc-200 transition-colors cursor-pointer'
                >
                    Cancel
                </button>
            </div>
        </div>
    );
}
