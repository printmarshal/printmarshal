import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The production build is written into the Go module so `go build` embeds it.
export default defineConfig({
  plugins: [react()],
  build: { outDir: '../internal/web/dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', ws: true },
      '/healthz': 'http://localhost:8080',
    },
  },
})
