# SafeRoute AI

SafeRoute AI is a safety-first navigation platform foundation for route planning, emergency contacts, and SOS events.

## Current implementation status
This repository is the initial source scaffold. Do not treat it as deployed or production-ready until required providers, database credentials, migrations, and deployment checks are completed.

## Architecture
- `web/`: React + TypeScript + Vite client.
- `api/`: Go HTTP API with health endpoint and structured configuration.
- `db/migrations/`: PostgreSQL schema for users, emergency contacts, saved places, route history, SOS events, and audit events.

## Local development
Prerequisites: Node.js 20+, Go 1.23+, PostgreSQL 15+.

1. Copy `.env.example` to `.env` and set actual values.
2. Apply `db/migrations/001_init.sql` to your PostgreSQL database.
3. Start API: `cd api && go run ./cmd/server`.
4. Start web: `cd web && npm install && npm run dev`.

Provider-backed routing requires a valid MapTiler key. SOS delivery integrations are not configured by default; the app must never claim an alert was delivered unless a real delivery provider confirms it.

## Safety and security
- SOS actions require explicit user confirmation.
- Route planning is informational and does not guarantee road access, safety, or emergency response.
- Never commit secrets. Use environment variables and deployment secret stores.
- Do not use this system to route emergency vehicles unless separately validated and authorized.
