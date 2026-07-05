# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

pdlife.app — log book / self-tracking app for dialysis patients (CAPD / APD / HD).

- **Stack:** Go 1.26 + Echo v4 + GORM + MySQL
- **Reference project:** nhe.one (`github.com/atiroop/nhe-app`) — same stack; follow its patterns wherever this repo is not explicit.

## Specs (read before implementing)

- [docs/auth_flow_spec.md](docs/auth_flow_spec.md) — two-step registration, email verification, roles (Admin/Member/Unverified), JWT + refresh token, log-book gating middleware
- [docs/schema_spec.md](docs/schema_spec.md) — users, patient_profiles, email_verifications, log book core/child tables

## Hard rules

- **`AutoMigrate` is disabled — never use it.** Every table gets its own SQL file in `migrations/` (run manually: `mysql -u USER -p DB_NAME < migrations/YYYYMMDD_name.sql`), and every GORM model gets an explicit `TableName()` method. Same discipline as nhe.one.
- `.env` is gitignored and holds real credentials — never commit it. `.env.example` documents the variables.

## Commands

```bash
go run .            # run locally (reads .env)
go build -o bin/pdlife .
```

## Deployment

- Server: Contabo VPS `109.123.233.155` (HestiaCP)
- Server path: `/home/pdlife/web/pdlife.app/public_html`
- Go service listens on **127.0.0.1:8085**
- nginx is already configured (`nginx.conf_go` / `nginx.ssl.conf_go` on the server) to proxy pdlife.app to that port — **do not touch nginx config**.
- Database: MySQL, `DB_NAME=pdlife_pdlife-db`, `DB_USER=pdlife_pdlife-db-admin`
