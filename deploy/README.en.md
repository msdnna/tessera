<p align="right"><a href="README.md">Русский</a> · <b>English</b></p>

# Tessera — production deployment

Single-box Docker deployment behind Caddy (automatic HTTPS). Images are built on
the dev box and shipped to the server as a tarball — the server never holds the
source tree or a build toolchain. (Later: push to GHCR and `docker pull` instead.)

```
┌────────── VDS (Ubuntu 24.04) ───────────┐
│  Caddy :80/:443  ──TLS──► frontend(nginx)│
│                            │ /api ─► backend (distroless) ─► postgres │
│                    └ /livekit ─► livekit (signalling, no host port)   │
│  livekit :7881/tcp + :7882/udp — call media only                      │
│  80/443 + media ports published; DB + backend internal-only           │
└──────────────────────────────────────────┘
```

## Files

| File | Where it runs | Purpose |
|------|---------------|---------|
| `build-and-save.sh` | dev box | build prod images → `dist/*.tar.gz` |
| `server-bootstrap.sh` | server (once) | OS hardening + Docker install |
| `docker-compose.yml` | server | image-based prod stack |
| `Caddyfile` | server | TLS edge + reverse proxy |
| `livekit.yaml` | server | video-call SFU config (no secrets — those live in `.env`) |
| `egress.yaml` | server | conference recording config (only needed with the `recording` profile) |
| `chrome-sandboxing-seccomp-profile.json` | server | seccomp profile for egress's Chrome sandbox (same) |
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
scp deploy/{docker-compose.yml,Caddyfile,livekit.yaml,.env.example} user@server:/opt/tessera/
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

### One-off: ownership of the uploads volume

Only for deployments created **before** the release that fixed #2820. The backend
runs as `nonroot` (65532), while Docker gave older `backend_uploads` volumes to
`root` — image and attachment uploads then fail with "permission denied". Fresh
volumes now fix themselves (the image ships the directory with the right owner);
an existing one needs a single manual pass:

```bash
docker volume ls                                   # find <project>_backend_uploads
docker compose stop backend
docker run --rm -v <project>_backend_uploads:/d busybox chown -R 65532:65532 /d
docker compose start backend
```

To verify, the backend log should carry `uploads: каталог доступен на запись` at
startup. A `uploads: каталог НЕ доступен на запись` line means the step is still
needed.

## Video calls (LiveKit)

Calls and meetings run through an **SFU** — the `livekit` container: every
participant uploads a single track to the server instead of one per peer.
Without it a four-way call is capped by the weakest participant's uplink.

**Firewall ports to open** (`ufw allow` — `server-bootstrap.sh` deliberately
leaves these closed so hosts without conferences don't expose them):

| Port | For | If closed |
|------|-----|-----------|
| `7882/udp` | all call media (one port, thanks to UDP mux) | calls fall back to TCP: higher latency, worse quality |
| `7881/tcp` | ICE over TCP — the fallback where UDP is blocked | participants behind strict corporate firewalls can't connect at all |

```bash
sudo ufw allow 7882/udp && sudo ufw allow 7881/tcp
```

The signalling port **7880 is not published**, and it should not be: it is
proxied by Caddy as `https://<DOMAIN>/livekit/*` on the certificate you already
have. That is not cosmetic — 7880 also serves LiveKit's management HTTP API
(create room, kick a participant, list who is present), which only our backend
is meant to call.

**Secrets.** `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` go in `.env`, and without
them `docker compose up` fails on purpose with an explicit error — there is no
default secret. The secret signs participant passes: whoever holds it can join
any meeting on this server. Treat it like `JWT_SECRET`.

**Updating an existing deployment.** Besides the new images, copy `livekit.yaml`
over and add the four `LIVEKIT_*` entries from `.env.example` to `.env` — the
stack will not start otherwise.

**Check after startup:** `docker compose logs livekit` shows `starting LiveKit
server`, and `curl -sf https://<DOMAIN>/livekit/` answers something (a protocol
error is fine) rather than 502.

## Conference recording (optional, off by default)

Recording is done by the **server**, not by a participant's browser: the `egress`
container joins the meeting as a hidden participant, renders the grid in a headless
Chrome and writes the mp4 straight into the `backend_uploads` volume
(`rec/<conference>/`), from where the ordinary download route serves it. Turn
nothing on and you pay nothing — both containers sit behind the `recording`
profile, and a plain `docker compose up -d` never starts them.

**Turning it on is one command, and it changes two things at once:**

```bash
LIVEKIT_REDIS_HOST=redis:6379 docker compose --profile recording up -d
```

Halves do not work, and they fail differently: the profile without the variable
starts an egress the SFU cannot reach, and the variable without the profile points
the SFU at a Redis that is not running, so it does not start at all. Turning it off
is the same command without either. Keep both halves together on every stack
update: `LIVEKIT_REDIS_HOST` in `.env` without the profile on the command line is
the same trap, merely deferred to the next `docker compose up -d`.

**The price, to be accepted deliberately.** Redis is not a cache here but the only
channel between `livekit-server` and egress: the server files a recording request
into `RedisStore` only — with the local store `getEgressStore` returns `nil`, there
is nowhere to put the request, and the recording silently never starts. There is no
way around it. And a non-empty `REDIS_HOST` puts the SFU into multi-node mode: room
state moves out of the process and into Redis, so a Redis outage takes down
**conferences as a whole**, not just recording. That is why recording is a
deployment option rather than always-on.

If recording is not enabled, the button does not disappear from the UI: the SFU
answers the backend with `egress not connected (redis required)` and the moderator
sees an error. The clean "recording is not configured" (503) happens only when
LiveKit itself is unconfigured.

**No new ports to open.** Egress only talks inside the compose network, and it
serves the grid templates to itself on 7980 inside the container — nothing leaves
the host, so no third party learns when you hold a meeting.

**Disk.** Budget roughly 1 GB per hour of a recorded room, all in the
`backend_uploads` volume (the same one as attachments — so the same backup).
Retention is per conference, set when scheduling it (`recording_ttl_days`, 30 days
by default; `0` means keep indefinitely), and expired files are deleted by a
background worker in the backend. A separate safety net is
`session_limits.file_output_max_duration: 2h` in `egress.yaml`: a recording nobody
pressed Stop on would otherwise run all night.

**Check after startup:** `docker compose --profile recording ps` — `tessera-egress`
is `Up` and `tessera-egress-init` is `Exited (0)` (it prepares the `rec/` directory
for its two writers — the backend and egress — once, and is not needed afterwards).
The real check is a live one: start recording in a running meeting, the red dot must
appear for **every** participant, and after Stop the recording shows up in the list
under the room within seconds. If it shows up as failed, look at `docker compose
logs egress` — it is quiet at `warn` by default, so errors are what you see.

## Backups (do this — confidentiality isn't complete without it)

```bash
# DB dump (cron, e.g. nightly)
docker compose exec -T postgres pg_dump -U tessera tessera | gzip > backup-$(date +%F).sql.gz
# attachments live in the backend_uploads volume — back that up too.
```
Encrypt dumps (`gpg`) and store them **off the box** (object storage / another
host). Test a restore periodically. Snapshot the VDS disk before each update.

## Behind an organization proxy

If Tessera sits behind a proxy/load-balancer you don't control (an org edge, an
API gateway), that proxy — not the bundled Caddy/nginx — governs latency and the
WebSocket. Getting the live board and snappy responses right there needs:

- **WebSocket upgrade** on `/api/ws`: forward `Upgrade` / `Connection` headers and
  use HTTP/1.1 to the upstream. Without it the realtime socket never connects and
  clients fall back to silent staleness + reconnect churn.
- **Idle timeout ≥ 60s** on that route (ideally minutes). The backend pings every
  25s to hold the socket open; a proxy that reaps idle connections faster than
  that will drop the board's live updates repeatedly.
- **Response compression** (`gzip`/`br`) for `application/json`, OR let the backend's
  own gzip pass through untouched (don't strip `Accept-Encoding` on the way in or
  `Content-Encoding` on the way out). Tessera gzips its JSON itself; the board /
  sync-journal payloads shrink ~10x, which is the difference between sub-second and
  multi-second loads on a constrained link.
- **Upstream keep-alive**: reuse connections to Tessera's frontend container rather
  than opening a fresh TCP + TLS per request — a board open fires ~10 calls at once
  and per-request connection setup dominates otherwise.

The bundled `frontend/nginx.conf` already does all of the above for the built-in
path; mirror those settings on the external proxy.

## Tuning Postgres for the host

`docker-compose.yml` ships conservative Postgres settings sized for a ~2GB box.
On a larger VDS, raise them in `.env` (then `docker compose up -d postgres`):

```
PG_SHARED_BUFFERS=1GB           # ~25% of RAM
PG_EFFECTIVE_CACHE_SIZE=3GB     # ~50-75% of RAM
PG_WORK_MEM=64MB
PG_MAINTENANCE_WORK_MEM=512MB
PG_RANDOM_PAGE_COST=1.1         # SSD; leave at 4 only for spinning disks
```

## Security posture (built in)

- Postgres, backend and LiveKit's signalling port (7880, which also carries its
  management API) have **no host ports** — internet-unreachable by design. Only
  LiveKit's media ports 7881/7882 face outward, and they serve nothing but the
  encrypted tracks of an already-authorised participant.
- Backend image is **distroless, non-root**, static binary.
- `APP_ENV=production` **fails closed** without `JWT_SECRET` / `ENCRYPTION_KEY` /
  `DATABASE_URL` / `PUBLIC_URL`.
- TLS everywhere via Caddy (auto-renewing Let's Encrypt).
- SSH: key-only, root login disabled, fail2ban; ufw allows only 22/80/443 (plus
  7881/tcp and 7882/udp if you enable calls — see the LiveKit section).
