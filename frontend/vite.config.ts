import { defineConfig } from 'vite';

// The Go service defaults to 127.0.0.1:8080. Keep auth headers in browser
// requests; the development proxy only solves same-origin access.
const target = process.env.DISK_API_TARGET || 'http://127.0.0.1:8080';

export default defineConfig({
  base: './',
  // CodeMirror is isolated in a lazy editor chunk and is never loaded for read-only browsing.
  build: { chunkSizeWarningLimit: 550 },
  server: {
    host: '127.0.0.1',
    proxy: {
      '/api': target,
      '/login': target,
      '/register': target,
    },
  },
});
