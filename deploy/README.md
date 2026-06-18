# Tessera — production deployment

Single-box Docker deployment behind Caddy (automatic HTTPS). Images are built on
the dev box and shipped to the server as a tarball — the server never holds the
source tree or a build toolchain. (Later: push to GHCR and `docker pull` instead.)

```
┌────────── VDS (Ubuntu 24.04) ───────────┐
│  Caddy :80/:443  ──TLS──► frontend(nginx)│
│                            │ /api ─► backend (distroless) ─► postgres │
│  only 80/443 published; DB + backend internal-only                    │
└──────────────────────────────────────────┘
```

## Files

| File | Where it runs | Purpose |
|------|---------------|---------|
| `build-and-save.sh` | dev box | build prod images → `dist/*.tar.gz` |
| `server-bootstrap.sh` | server (once) | OS hardening + Docker install |
| `docker-compose.yml` | server | image-based prod stack |
| `Caddyfile` | server | TLS edge + reverse proxy |
| `.env.example` | server | copy to `.env`, fill secrets |

## First-time deploy

**0. DNS** — point an A record (`tessera.example.com`) at the VDS IP before step 4
(Caddy needs it resolvable to issue the cert).

**1. Server prep** (on the VDS, sudo user):
```bash
scp deploy/server-bootstrap.sh user@server:/tmp/
ssh user@server 'sudo bash /tmp/server-bootstrap.sh'
# log out/in so the docker group applies
```

**2. Build + ship images** (dev box):
```bash
bash deploy/build-and-save.sh
scp deploy/dist/tessera-images-*.tar.gz user@server:/opt/tessera/
scp deploy/{docker-compose.yml,Caddyfile,.env.example} user@server:/opt/tessera/
```

**3. Configure** (server, `/opt/tessera`):
```bash
cp .env.example .env && chmod 600 .env
# generate secrets:
#   openssl rand -hex 32      (JWT_SECRET, ENCRYPTION_KEY)
#   openssl rand -base64 24   (POSTGRES_PASSWORD)
nano .env          # set DOMAIN, ACME_EMAIL, PUBLIC_URL, secrets, image tags
docker load -i tessera-images-*.tar.gz
```

**4. Launch**:
```bash
docker compose up -d
docker compose exec backend /migrate      # apply DB migrations
docker compose logs -f                     # watch Caddy get its cert
```

Open `https://tessera.example.com` and **register immediately** — the first user
becomes admin.

## Updating to a new version

```bash
# dev box
bash deploy/build-and-save.sh
scp deploy/dist/tessera-images-*.tar.gz user@server:/opt/tessera/
# server
docker load -i tessera-images-*.tar.gz
# bump BACKEND_IMAGE / FRONTEND_IMAGE tags in .env to the new versions
docker compose up -d
docker compose exec backend /migrate       # if the release added migrations
```

## Backups (do this — confidentiality isn't complete without it)

```bash
# DB dump (cron, e.g. nightly)
docker compose exec -T postgres pg_dump -U tessera tessera | gzip > backup-$(date +%F).sql.gz
# attachments live in the backend_uploads volume — back that up too.
```
Encrypt dumps (`gpg`) and store them **off the box** (object storage / another
host). Test a restore periodically. Snapshot the VDS disk before each update.

## Security posture (built in)

- Postgres + backend have **no host ports** — internet-unreachable by design.
- Backend image is **distroless, non-root**, static binary.
- `APP_ENV=production` **fails closed** without `JWT_SECRET` / `ENCRYPTION_KEY` /
  `DATABASE_URL` / `PUBLIC_URL`.
- TLS everywhere via Caddy (auto-renewing Let's Encrypt).
- SSH: key-only, root login disabled, fail2ban; ufw allows only 22/80/443.
