import { useState } from 'react';
import type { WatchEvent } from '../api';
import { editPlexWatchDate } from '../api';
import { ratingDisplay } from '../utils';

function Checkmark({ className }: { className?: string }) {
    return (
        <svg
            xmlns='http://www.w3.org/2000/svg'
            viewBox='0 0 20 20'
            fill='currentColor'
            className={className ?? 'w-4 h-4'}
        >
            <path
                fillRule='evenodd'
                d='M16.704 4.153a.75.75 0 0 1 .143 1.052l-8 10.5a.75.75 0 0 1-1.127.075l-4.5-4.5a.75.75 0 0 1 1.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 0 1 1.05-.143Z'
                clipRule='evenodd'
            />
        </svg>
    );
}

interface WatchTableProps {
    events: WatchEvent[];
    importStatuses?: Record<number, string> | null;
    onDateEdited?: () => void;
}

export default function WatchTable({
    events,
    importStatuses,
    onDateEdited,
}: WatchTableProps) {
    const [editingId, setEditingId] = useState<number | null>(null);
    const [editDate, setEditDate] = useState('');
    const [saving, setSaving] = useState(false);

    const startEdit = (e: WatchEvent) => {
        setEditingId(e.id);
        // Pre-fill with the current watched date (YYYY-MM-DD)
        setEditDate(e.plexWatchedAt?.slice(0, 10) || e.watchedOn || '');
    };

    const cancelEdit = () => {
        setEditingId(null);
        setEditDate('');
    };

    const saveEdit = async (e: WatchEvent) => {
        if (!e.plexActivityId || !editDate) return;
        setSaving(true);
        try {
            await editPlexWatchDate(e.plexActivityId, editDate);
            setEditingId(null);
            setEditDate('');
            onDateEdited?.();
        } catch (err) {
            console.error('Failed to update date:', err);
        } finally {
            setSaving(false);
        }
    };
    if (events.length === 0) {
        return (
            <p className='text-zinc-500 text-sm text-center py-8'>
                No entries yet. Fetch your Plex history or Letterboxd diary to
                populate the table.
            </p>
        );
    }

    return (
        <div className='overflow-x-auto'>
            <table className='w-full text-sm'>
                <thead>
                    <tr className='border-b border-zinc-700 text-zinc-400 text-left'>
                        <th className='pb-2 pr-4 font-medium'>Title</th>
                        <th className='pb-2 pr-4 font-medium w-18'>Year</th>
                        <th className='pb-2 pr-4 font-medium w-24'>Rating</th>
                        <th className='pb-2 pr-4 font-medium w-28'>Watched</th>
                        <th className='pb-2 pr-4 font-medium w-16 text-center'>
                            Rewatch
                        </th>
                        <th className='pb-2 pr-2 font-medium w-24 text-center'>
                            Plex
                        </th>
                        <th className='pb-2 font-medium w-20 text-center'>
                            Letterboxd
                        </th>
                    </tr>
                </thead>
                <tbody>
                    {events.map((e) => {
                        const status = importStatuses?.[e.id];
                        return (
                            <tr
                                key={e.id}
                                className='border-b border-zinc-800 hover:bg-zinc-800/40 transition-colors'
                            >
                                <td
                                    className='py-2 pr-4 font-medium max-w-xs truncate'
                                    title={e.plexRatingKey || undefined}
                                >
                                    {e.title}
                                    {status && (
                                        <span
                                            className={`ml-2 text-xs font-normal ${
                                                status === 'imported'
                                                    ? 'text-green-400'
                                                    : status === 'duplicate'
                                                      ? 'text-yellow-400'
                                                      : 'text-zinc-500'
                                            }`}
                                        >
                                            {status}
                                        </span>
                                    )}
                                </td>
                                <td className='py-2 pr-4 text-zinc-400'>
                                    {e.year || '—'}
                                </td>

                                <td className='py-2 pr-4 text-yellow-400'>
                                    {ratingDisplay(e.rating) || '—'}
                                </td>
                                <td className='py-2 pr-4 text-zinc-400'>
                                    {editingId === e.id ? (
                                        <span className='flex items-center gap-1'>
                                            <input
                                                type='date'
                                                value={editDate}
                                                onChange={(ev) =>
                                                    setEditDate(ev.target.value)
                                                }
                                                disabled={saving}
                                                className='bg-zinc-800 border border-zinc-600 rounded px-1.5 py-0.5 text-xs text-zinc-200 focus:outline-none focus:border-blue-500'
                                            />
                                            <button
                                                onClick={() => saveEdit(e)}
                                                disabled={saving || !editDate}
                                                className='text-green-400 hover:text-green-300 disabled:opacity-50 cursor-pointer'
                                                title='Save'
                                            >
                                                <svg
                                                    xmlns='http://www.w3.org/2000/svg'
                                                    viewBox='0 0 20 20'
                                                    fill='currentColor'
                                                    className='w-4 h-4'
                                                >
                                                    <path
                                                        fillRule='evenodd'
                                                        d='M16.704 4.153a.75.75 0 0 1 .143 1.052l-8 10.5a.75.75 0 0 1-1.127.075l-4.5-4.5a.75.75 0 0 1 1.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 0 1 1.05-.143Z'
                                                        clipRule='evenodd'
                                                    />
                                                </svg>
                                            </button>
                                            <button
                                                onClick={cancelEdit}
                                                disabled={saving}
                                                className='text-zinc-500 hover:text-zinc-300 disabled:opacity-50 cursor-pointer'
                                                title='Cancel'
                                            >
                                                <svg
                                                    xmlns='http://www.w3.org/2000/svg'
                                                    viewBox='0 0 20 20'
                                                    fill='currentColor'
                                                    className='w-4 h-4'
                                                >
                                                    <path d='M6.28 5.22a.75.75 0 0 0-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 1 0 1.06 1.06L10 11.06l3.72 3.72a.75.75 0 1 0 1.06-1.06L11.06 10l3.72-3.72a.75.75 0 0 0-1.06-1.06L10 8.94 6.28 5.22Z' />
                                                </svg>
                                            </button>
                                        </span>
                                    ) : (
                                        <span
                                            className={
                                                e.plexActivityId
                                                    ? 'cursor-pointer hover:text-zinc-200 transition-colors'
                                                    : ''
                                            }
                                            onClick={() =>
                                                e.plexActivityId && startEdit(e)
                                            }
                                            title={
                                                e.plexActivityId
                                                    ? 'Click to edit date'
                                                    : undefined
                                            }
                                        >
                                            {e.watchedOn || '—'}
                                        </span>
                                    )}
                                </td>
                                <td className='py-2 pr-4 text-center'>
                                    {e.rewatch ? (
                                        <Checkmark className='w-4 h-4 text-blue-400 inline' />
                                    ) : (
                                        <span className='text-zinc-700'>—</span>
                                    )}
                                </td>
                                <td className='py-2 pr-2 text-center'>
                                    {e.inPlex ? (
                                        <Checkmark className='w-4 h-4 text-orange-500 inline' />
                                    ) : (
                                        <span className='text-zinc-700'>—</span>
                                    )}
                                </td>
                                <td className='py-2 text-center'>
                                    {e.inLb ? (
                                        <Checkmark className='w-4 h-4 text-green-400 inline' />
                                    ) : (
                                        <span className='text-zinc-700'>—</span>
                                    )}
                                </td>
                            </tr>
                        );
                    })}
                </tbody>
            </table>
        </div>
    );
}
