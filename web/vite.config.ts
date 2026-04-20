import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: '/app/',
  build: {
    outDir: '../cmd/ct/assets/web',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:5737',
      '/ws': {
        target: 'ws://127.0.0.1:5737',
        ws: true,
      },
    },
  },
});