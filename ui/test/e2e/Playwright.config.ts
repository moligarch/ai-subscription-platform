import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './specs',
  use: {
    headless: true,
    baseURL: process.env.E2E_BASE_URL || 'http://localhost:5173'
  },
  timeout: 30_000
});
