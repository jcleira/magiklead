# 06 — Frontend on Hetzner

**Type**: AFK — mechanical extension of #5 once the deploy infra exists.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#5 — Hetzner VPS + domain + api online](./05-hetzner-vps-and-api.md)

## What to build

Add the Next.js frontend service to the production docker-compose stack on the same Hetzner VPS, with Caddy routing the apex domain to it. After this slice, visiting `https://<domain>` shows the marketing page, and the sign-in flow leads through Clerk to the app shell — fully off Vercel, running on operator-controlled infrastructure.

## Acceptance criteria

- [ ] `docker-compose.prod.yml` extended with a `frontend` service running `next start` on port 3000.
- [ ] Frontend `Dockerfile` (production variant — distinct from `Dockerfile.dev`) builds `next build` with `output: "standalone"` so the runtime image is minimal.
- [ ] Caddyfile routes the apex domain (e.g. `magiklead.com` and `www.magiklead.com`) to the frontend container with auto-LetsEncrypt.
- [ ] Production frontend env file captures `NEXT_PUBLIC_API_URL=https://api.<domain>` and the dev Clerk publishable key (production Clerk swap is a follow-up PRD).
- [ ] `curl -L https://<domain>` returns the marketing landing page HTML (200 OK).
- [ ] Browser flow on a fresh user: visit `https://<domain>` → sign in via Clerk hosted page → land on the app shell. No 4xx/5xx errors in the network tab.
- [ ] No Vercel project still serves traffic for the domain. Vercel project archived or deleted (operator's preference).

## Modules touched

- Production deploy module (extended `docker-compose.prod.yml` + `Caddyfile`).

## Test prior art

- The dev `devpod/compose.yml` runs the frontend in dev mode behind Traefik. The prod variant runs `next start` behind Caddy — same shape, different dev-vs-prod knobs.

## Out of scope

- Production Clerk swap — out of scope per PRD; site uses dev Clerk during validation window.
- Frontend SSR optimisation, image CDN, edge caching — defer; v1 is single-VPS-renders-everything.
- Dropping Vercel-specific code (`@vercel/*` packages) — not required for the deploy to work, but worth a follow-up cleanup.
