# Krovara web (SvelteKit + adapter-static)

Pure SPA. Talks to the Go API via `/api` (dev: Vite proxy → `localhost:8080`).

## Dev

```bash
cd web
pnpm install
pnpm dev          # http://localhost:5173
```

The Go API must also be running for auth flows to work. The dev Vite proxy
forwards `/api/*` and `/internal/*` to `localhost:8080`.

## Build

```bash
pnpm build        # outputs to web/build/
pnpm preview      # serve the build locally
```

In prod (session 15) Nginx serves `web/build/` directly.

## Conventions

- File-based routing (`src/routes/`).
- Pure SPA: `ssr=false` + `prerender=false` in `+layout.ts`.
- Tailwind v4 via `@tailwindcss/vite` (no `tailwind.config.js` needed; tweak via `@import "tailwindcss"` directives).
- Auth state: `src/lib/stores/auth.ts`. Access token in memory, refresh token in localStorage (ADR-009 — to be migrated to httpOnly cookie in session 15).
- API client: `src/lib/api.ts`. Transparent refresh on 401.

## Session 12+ will

- Replace the placeholder `/app` with the actual spaces/channels UI.
- Add the XMPP messaging client (`@xmpp/client` or hand-rolled).
