interface DeleteConfirmModalProps {
    onConfirm: () => void;
    onCancel: () => void;
}

export default function DeleteConfirmModal({
    onConfirm,
    onCancel,
}: DeleteConfirmModalProps) {
    return (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/60'>
            <div className='bg-zinc-900 border border-zinc-700 rounded-lg p-6 w-120 shadow-xl'>
                <h3 className='text-lg font-semibold mb-2'>Delete Database</h3>
                <p className='text-sm text-zinc-400 mb-4'>
                    This will permanently delete all watch history data. This
                    action cannot be undone.
                </p>
                <div className='flex gap-2'>
                    <button
                        onClick={onCancel}
                        className='flex-1 px-4 py-2 bg-zinc-700 hover:bg-zinc-600 rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        Cancel
                    </button>
                    <button
                        onClick={onConfirm}
                        className='flex-1 px-4 py-2 bg-red-700 hover:bg-red-600 rounded text-sm font-medium transition-colors cursor-pointer'
                    >
                        Delete
                    </button>
                </div>
            </div>
        </div>
    );
}
