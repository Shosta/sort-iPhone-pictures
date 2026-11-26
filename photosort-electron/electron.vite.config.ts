import { resolve } from 'node:path'
import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import react from '@vitejs/plugin-react'
import { resolveCommit } from './scripts/build-info.mjs'

export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
    define: { __PHOTOSORT_COMMIT__: JSON.stringify(resolveCommit()) }
  },
  preload: { plugins: [externalizeDepsPlugin()] },
  renderer: {
    root: resolve('src/renderer'),
    build: { rollupOptions: { input: resolve('src/renderer/index.html') } },
    plugins: [react()]
  }
})
