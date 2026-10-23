# SafeRoute AI

Safety-first navigation web app with a Go API, PostgreSQL persistence, provider-backed geocoding/directions, user accounts, saved places, route history, emergency contacts and GPS SOS event records.

## Implemented

- Responsive React + TypeScript web application.
- Go REST API with health/status endpoints, CORS allow-list, security headers and request timeouts.
- PostgreSQL schema applied automatically at API startup using versioned, embedded SQL migrations.
- Registration/login with bcrypt password hashing, random session tokens stored as SHA-256 hashes, seven-day expiry and logout.
- Account-scoped emergency contacts, saved places, route history and SOS event history.
- MapTiler geocoding and directions through the backend. Route results draw on the Leaflet map and show distance, ETA and available steps.
- Explicit five-second SOS confirmation followed by a separate user confirmation and GPS request. The API records an event; it does not claim contacts have been notified.
- GitHub Actions web build and Go test/build workflow.
- Vercel SPA config under `web/vercel.json`; Render API blueprint at `render.yaml`.

## Provider and safety limits

- Configure a MapTiler API key to use real tiles, geocoding and directions. A truck-specific profile is deliberately rejected instead of returning a car route labelled as truck-safe.
- Map data may omit temporary closures, access rules, height/weight restrictions and other vehicle-specific limitations. This is not a validated emergency dispatch/navigation service.
- SOS delivery to SMS/voice contacts is **not configured**. A database event is not an emergency notification. No paid messaging provider is activated by this repository.
- Password reset uses one-time, hashed, 30-minute tokens and revokes existing sessions. Email delivery requires a verified Resend sender plus `RESEND_API_KEY`, `MAIL_FROM`, and `PASSWORD_RESET_URL`; without these settings, the API returns an explicit configuration error instead of claiming an email was sent.
- The frontend uses browser session storage for the session token. For a high-assurance production deployment, move sessions to Secure, HttpOnly, SameSite cookies and add CSRF protection.
- No secrets belong in Git or in client code. Restrict the public MapTiler key by allowed website origins and API scopes.

## Configuration

### API (Render or local)

Required for persisted functionality:
- `DATABASE_URL`: PostgreSQL connection string.
- `CORS_ORIGINS`: comma-separated exact frontend origins, e.g. `http://localhost:5173,https://your-web-project.vercel.app`.
- `MAPTILER_API_KEY`: server-side MapTiler key for geocoding and directions.
- `RESEND_API_KEY`, `MAIL_FROM`, `PASSWORD_RESET_URL`: optional password-reset email delivery settings. Use a verified sender and your actual frontend origin.
- `PORT`: provided by Render; local default is `8080`.

The API automatically applies the embedded migrations at startup. A valid PostgreSQL service must be available first. The Render blueprint marks credentials as unsynced secrets; enter them in Render's environment settings yourself. Do not deploy until the database plan and cost are explicitly acceptable to you.

### Web (Vercel or local)

Set:
- `VITE_API_BASE_URL`: deployed API base URL, without a trailing slash.
- `VITE_MAPTILER_KEY`: restricted public key for browser map tiles.

For Vercel, set the project root directory to `web`, then add both variables in Vercel project environment settings and deploy. For local development, set these variables in the web process environment and run `npm install && npm run dev` in `web`.

## Local development

Prerequisites: Node.js 20+, Go 1.23+, PostgreSQL 15+.

1. Start PostgreSQL and set `DATABASE_URL`, `CORS_ORIGINS`, and `MAPTILER_API_KEY`.
2. Start the API: `cd api && go run ./cmd/server`. It applies SQL migrations at startup.
3. In another terminal, set `VITE_API_BASE_URL=http://localhost:8080` and `VITE_MAPTILER_KEY`, then run `cd web && npm install && npm run dev`.
4. The app reports actual API/database status. If the provider key or database is missing, the related functions return an explicit configuration error.

## Verification

GitHub Actions builds the frontend and runs Go tests/build on pushes and pull requests. A successful build is not proof of provider credentials, deployment, route coverage or SOS delivery; those require live integration checks.
