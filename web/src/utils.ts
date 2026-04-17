export function ratingDisplay(rating: number): string {
    if (!rating) return '';
    const stars = rating / 2;
    const full = Math.floor(stars);
    const half = stars % 1 >= 0.5;
    return '\u2605'.repeat(full) + (half ? '\u00BD' : '');
}

export function formatTs(iso: string): string {
    try {
        return new Date(iso).toLocaleString('en-US', {
            month: '2-digit',
            day: '2-digit',
            year: 'numeric',
            hour: 'numeric',
            minute: '2-digit',
            hour12: true,
        });
    } catch {
        return iso;
    }
}
