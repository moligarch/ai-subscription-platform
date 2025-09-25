# E2E notes

- Ensure backend running (http://localhost:8080) and CORS/proxy is set (Vite proxy handles this).
- Ensure a deterministic admin API key exists in DB; set env var when running e2e:
  `E2E_ADMIN_KEY=your_key npm run e2e`
