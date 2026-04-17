import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// https://vite.dev/config/
export default defineConfig({
    plugins: [react(), tailwindcss()],
    build: {
        // Output directly into the Go embed directory.
        outDir: '../cmd/server/dist',
        emptyOutDir: true,
    },
    server: {
        proxy: {
            '/api': 'http://localhost:12349',
        },
    },
});
