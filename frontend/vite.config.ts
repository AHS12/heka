/// <reference types="vitest/config" />
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

// SPEC-12 §2/§3: Tailwind v4 via its Vite plugin (replaces the template's
// PostCSS setup); `wailsjs` aliased because the bindings are generated at
// build time (vi.mock'ed in tests, mapped for tsc via tsconfig paths).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@wailsjs': path.resolve(__dirname, 'wailsjs'),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})