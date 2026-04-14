# Elevenlabs-Go-Grpc-Sqlc Monorepo Setup Guide

This repository contains four runtime services:

- Auth service (ConnectRPC + PostgreSQL + Redis)
- Voice service (ConnectRPC + PostgreSQL + Redis + S3)
- Generation service (ConnectRPC + PostgreSQL + Redis + S3 + Modal TTS)
- Web app (Next.js)

It also contains one deployment-only service:

- TTS service on Modal (`services/tts/chatterbox_tts.py`)

## Architecture and Local Ports

- Auth API: `http://localhost:50051`
- Voice API: `http://localhost:50052`
- Generation API: `http://localhost:50053`
- Web UI: `http://localhost:3000`
- Auth Postgres (Docker): `localhost:5433`
- Voice Postgres (Docker): `localhost:5434`
- Generation Postgres (Docker): `localhost:5435`
- Redis (Docker): `localhost:6379`
- RedisInsight (Docker): `http://localhost:5540`

## Prerequisites

- Go 1.25+
- Docker + Docker Compose
- Node.js 20+ and pnpm
- Goose CLI (`go install github.com/pressly/goose/v3/cmd/goose@latest`)
- sqlc CLI (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)

Optional but recommended:

- `grpcurl` for API testing
- Modal CLI for TTS deployment (`pip install modal`)

## 1. Start Infrastructure with Docker Compose

From repository root:

```bash
docker compose up -d
```

Docker Compose uses the defaults defined in `docker-compose.yml`, so no root `.env` file is required for the standard local setup.

If you want to override the database or Redis ports/passwords, you can pass them inline when running Compose or add a root `.env` later, but the normal workflow in this repository is to keep configuration in the service-local `.env` files.

Stop infrastructure:

```bash
docker compose down
```

## 2. Configure Environment Files Per Service

Create `.env` files from examples:

```bash
cp services/auth/.env.example services/auth/.env
cp services/voice/.env.example services/voice/.env
cp services/generation/.env.example services/generation/.env
cp web/.env.example web/.env
```

On Windows PowerShell:

```powershell
Copy-Item services/auth/.env.example services/auth/.env
Copy-Item services/voice/.env.example services/voice/.env
Copy-Item services/generation/.env.example services/generation/.env
Copy-Item web/.env.example web/.env
```

### Required Env Alignment Across Services

- Use the same `JWT_SECRET` in auth, voice, generation, and web.
- Keep S3 values consistent across voice, generation, and web:
  - `S3_ENDPOINT`
  - `S3_REGION`
  - `S3_BUCKET`
  - `S3_ACCESS_KEY_ID`
  - `S3_SECRET_ACCESS_KEY`
- `GENERATION_TTS_ENDPOINT` in generation must point to deployed Modal endpoint `/generate`.
- `GENERATION_TTS_API_KEY` in generation must match `CHATTERBOX_API_KEY` in Modal secret `chatterbox-multi-tts`.

## 3. Run Database Migrations

From repository root:

```bash
make migrate-up
```

This runs goose per service using each service-local `.env`:

- `services/auth/db/migrations`
- `services/voice/db/migrations`
- `services/generation/db/migrations`

Useful migration commands:

```bash
make migrate-status
make migrate-down
make migrate-reset
```

## 4. (Optional) Generate Proto and SQL Code

If you changed proto or SQL queries:

```bash
make proto
make sqlc
```

## 5. Deploy or Verify TTS on Modal

Generation depends on the TTS endpoint for speech jobs. Deploy TTS first (or verify existing endpoint):

```bash
modal deploy services/tts/chatterbox_tts.py
```

TTS setup and required Modal secrets are documented in:

- `services/tts/readme.md`

## 6. Start Each Backend Microservice

From repository root, run each in a separate terminal:

```bash
make run-auth
make run-voice
make run-gen
```

Or run directly:

```bash
cd services/auth && go run ./cmd/
cd services/voice && go run ./cmd/
cd services/generation && go run ./cmd/
```

## 7. Seed System Voices (Recommended)

After voice service DB/S3 config is ready:

```bash
make seed-voices
```

## 8. Configure and Run Web App

In `web/.env`, set backend URLs (or rely on these defaults):

```env
NEXT_PUBLIC_AUTH_RPC_URL=http://localhost:50051
NEXT_PUBLIC_VOICE_RPC_URL=http://localhost:50052
NEXT_PUBLIC_GENERATION_RPC_URL=http://localhost:50053
```

Install and run web:

```bash
cd web
pnpm install
pnpm dev
```

Open `http://localhost:3000`.

## 9. Suggested Startup Order

1. Docker Compose infra (Postgres + Redis)
2. TTS endpoint deployed on Modal and reachable
3. Migrations (`make migrate-up`)
4. Auth, Voice, Generation services
5. Voice seed job (`make seed-voices`)
6. Web app (`pnpm dev`)

## 10. Quick Health Checks

- Infra logs:

```bash
make docker-logs
```

- Migration status:

```bash
make migrate-status
```

- Confirm ports are listening:
  - `50051` auth
  - `50052` voice
  - `50053` generation
  - `3000` web

## Common Issues

- Generation jobs stuck or failing:
  - Check `GENERATION_TTS_ENDPOINT` and `GENERATION_TTS_API_KEY`.
  - Ensure Modal TTS service accepts `x-api-key` matching generation config.
- `token has been revoked` or auth failures:
  - Ensure all services and web share the same `JWT_SECRET`.
- S3 upload/download errors:
  - Re-check endpoint, region, bucket, and key/secret in voice, generation, and web.
- Migration hits wrong DB:
  - Use per-service `.env` values and run migrations via existing Make targets.
