# Admin UI (Svelte) — Quickstart

Defaults:
- Dev server: http://localhost:5173
- Backend proxy target: http://localhost:8080 (set VITE_BACKEND_URL to change)

Install deps:
```bash
cd ui
npm ci
```

Dev:
```
# run backend first (see project README)
# then:
npm run dev
# open http://localhost:5173
```

Build:
```
npm run build
# copy ui/dist -> deploy/admin-ui (we provide a Makefile target in repo root)
```


Tests:

- Unit tests: `npm run test:unit`
- E2E (Playwright): `npm run e2e` (requires backend running and an admin API key)

Seeding an admin API key for e2e:

- Use your existing seeder (e.g. `cmd/e2e-setup`) or create a stable admin API key in DB.

- E2E expects to login with that key via the login page. Set the test key in `ui/test/e2e/env` or follow prompts.

Notes:

- To change backend in dev use `.env` in `ui/`:
- `VITE_BACKEND_URL=http://localhost:8080`