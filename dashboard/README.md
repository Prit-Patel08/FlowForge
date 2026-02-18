# Agent-Sentry Dashboard

Next.js frontend for the local Agent-Sentry API.

## Setup

1. Copy environment defaults:

```bash
cp .env.example .env.local
```

2. Install dependencies:

```bash
npm ci
```

3. Start frontend:

```bash
npm run dev
```

4. Start backend API from repo root:

```bash
go run . dashboard
```

## Production Build

```bash
npm run build
npm run start
```

The build uses webpack (`next build --webpack`) to avoid Turbopack sandbox process-binding issues in restricted environments.

## Security Notes

- API base URL is configured by `NEXT_PUBLIC_SENTRY_API_BASE`.
- API key is entered in the UI and stored only in browser session storage.
- Security headers and a CSP are defined in `next.config.ts`.
- Dashboard now includes:
  - Incident timeline (`/timeline`)
  - Reason-for-action panel
  - Confidence breakdown (CPU score, entropy score, confidence score)
