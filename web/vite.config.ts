import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [
    vue({
      template: {
        compilerOptions: {
          isCustomElement: (tag) => tag.startsWith('wiki-'),
        },
      },
    }),
  ],
  build: {
    outDir: '../internal/server/static/dist',
    emptyOutDir: true,
    // SSR article text should win bandwidth on slow links. Static imports are
    // discovered from the entry module after the blocking stylesheet arrives.
    modulePreload: false,
    // Keep the manifest outside Vite's hidden .vite directory so Go embed
    // includes it in the production binary.
    manifest: 'manifest.json',
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
