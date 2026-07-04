# Deploying to a Raspberry Pi

FaturaCloud ships as a single multi-arch Docker image
(`ghcr.io/mamissaoui/fatura-cloud`, built for `linux/amd64` and `linux/arm64`),
so a 64-bit Raspberry Pi can run it without building anything from source.

## Prerequisites

- Raspberry Pi 3/4/5 running **64-bit** Raspberry Pi OS. Confirm with:
  ```bash
  uname -m
  # aarch64 -> good. armv7l means a 32-bit OS; re-image with the 64-bit build first.
  ```
- Docker Engine + the Compose plugin:
  ```bash
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker $USER   # log out/in for this to take effect
  ```

## 1. Authenticate to GHCR

The image is currently **private**. On the Pi, log in with a GitHub Personal
Access Token that has at least the `read:packages` scope (Settings → Developer
settings → Personal access tokens on github.com):

```bash
docker login ghcr.io -u MaMissaoui
# paste the PAT as the password
```

## 2. Get the compose file

Copy `docker-compose.yml` from this repo onto the Pi (scp, git clone, or paste
it directly) into its own directory, e.g. `~/fatura-cloud/`.

## 3. Configure secrets

The container treats the presence of the `/data` volume as "this is a real
deployment" and **refuses to start** unless `JWT_SECRET` and `ADMIN_PASSWORD`
are set — it will not silently fall back to the insecure defaults. Create a
`.env` file next to `docker-compose.yml`:

```bash
cat > .env <<EOF
JWT_SECRET=$(openssl rand -hex 32)
ADMIN_EMAIL=you@example.com
ADMIN_PASSWORD=$(openssl rand -base64 18)
EOF
chmod 600 .env
```

Save the generated `ADMIN_PASSWORD` somewhere safe — it's only used to seed the
first admin user on first startup, and isn't recoverable from the file after
you close the terminal unless you kept a copy.

## 4. Pull and start

```bash
docker compose pull
docker compose up -d
```

Compose resolves the multi-arch tag automatically — the Pi pulls the `arm64`
image without any extra flags. Check it came up cleanly:

```bash
docker compose logs -f app
```

The app is now reachable at `http://<pi-ip>:8080`. Log in with the admin email
and password from your `.env`.

## Updating to a new version

```bash
docker compose pull
docker compose up -d
```

To pin a specific version instead of always tracking `latest`, set `VERSION`
in `.env` (e.g. `VERSION=v1.1.0`) — the compose file reads
`ghcr.io/mamissaoui/fatura-cloud:${VERSION:-latest}`.

## Data and backups

All state (SQLite database) lives in the `fatura_data` named volume, mounted
at `/data` in the container — it persists across `docker compose pull`/`up`
cycles and image updates. FaturaCloud also has a built-in backup feature
(Settings → Backup, admin only) that snapshots the database to a configurable
schedule; download those snapshots off the Pi periodically since they still
live on the same disk as the volume.

## Optional: OIDC single sign-on

FaturaCloud can authenticate against Authelia (or any standards-compliant
OIDC provider) instead of — or alongside — local email/password login. Local
login always remains available as a fallback. See
[`docs/oidc-sso.md`](docs/oidc-sso.md) for the full design, the `OIDC_*`
environment variables, the Authelia-side client registration steps, and the
security model. This is unset (off) by default, so nothing here changes
unless you deliberately configure it.

## Troubleshooting

- **Container exits immediately on first run**: check `docker compose logs
  app` — it's almost always a missing `JWT_SECRET`/`ADMIN_PASSWORD` in `.env`
  (see step 3).
- **`docker compose pull` fails with "unauthorized"**: the GHCR login (step 1)
  expired or wasn't run on this host — re-run `docker login ghcr.io`.
- **Wrong architecture pulled / "exec format error"**: confirm `uname -m`
  reports `aarch64`, not `armv7l` — a 32-bit OS isn't covered by the current
  image build.
