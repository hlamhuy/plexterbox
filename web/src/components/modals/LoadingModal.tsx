interface LoadingModalProps {
    message: string;
}

export default function LoadingModal({ message }: LoadingModalProps) {
    return (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/60'>
            <div className='bg-zinc-900 border border-zinc-700 rounded-lg p-8 shadow-xl flex flex-col items-center gap-4'>
                <svg
                    className='w-8 h-8 text-zinc-400 animate-spin'
                    xmlns='http://www.w3.org/2000/svg'
                    fill='none'
                    viewBox='0 0 24 24'
                >
                    <circle
                        className='opacity-25'
                        cx='12'
                        cy='12'
                        r='10'
                        stroke='currentColor'
                        strokeWidth='4'
                    />
                    <path
                        className='opacity-75'
                        fill='currentColor'
                        d='M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z'
                    />
                </svg>
                <p className='text-sm text-zinc-300'>{message}</p>
            </div>
        </div>
    );
}
