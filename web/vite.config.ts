import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api':      { target: 'http://localhost:8082', changeOrigin: true },
      '/internal': { target: 'http://localhost:8082', changeOrigin: true },
      '/xmpp-websocket': { target: 'ws://localhost:5280', ws: true, changeOrigin: true },
      '/voip/ws': { target: 'ws://localhost:8083', ws: true, changeOrigin: true },
      '/ntfy': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        rewrite: (p: string) => p.replace(/^\/ntfy/, '')
      }
    }
  }
});
