import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Vite config for the Mule frontend.
//
// Migrated from Create React App (react-scripts) to Vite to clear the
// Dependabot advisories that were locked to CRA's deprecated build toolchain.
//
// `build.outDir` is kept as "build" so the existing Go embed
// (internal/frontend/build) and CI artifact flow continue to work unchanged.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': {
        target: 'http://localhost:8080',
        ws: true,
      },
    },
  },
  build: {
    outDir: 'build',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/setupTests.js',
    globals: true,
    passWithNoTests: true,
  },
});
