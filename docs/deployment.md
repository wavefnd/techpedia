# Deploy a server

## Requirements and source

Use Docker Engine and Compose v2 with persistent local disk. Budget for source,
build layers, databases and media, not only the final image.

```sh
git clone --branch main https://github.com/wavefnd/techpedia.git
cd techpedia
cp .env.example .env
chmod 600 .env
```

Use the official GitHub repository and `main`. Do not download a similarly named
third-party repository.

## HTTPS with bundled Caddy

Point DNS at the server and edit `.env`:

```dotenv
TECHPEDIA_ENABLE_HTTPS=true
TECHPEDIA_DOMAIN=wiki.example.com
TECHPEDIA_PUBLIC_URL=https://wiki.example.com
TECHPEDIA_TLS_EMAIL=operator@example.com
TECHPEDIA_HTTP_BIND=127.0.0.1
TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME=your_owner
TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD=your-own-long-random-password
TECHPEDIA_MCP_ENABLED=false
```

Replace samples, open inbound 80/443, and keep 8080 private:

```sh
./start.sh
docker compose --profile production ps
docker compose logs --tail=100 techpedia
```

The script builds, starts the profile and checks `/api/health`. Configure editions
and a main page after signing in. A new server starts with empty runtime data;
it does not copy a developer's wiki.

## Existing reverse proxy

Disable bundled HTTPS mode and keep the backend host binding at loopback. Forward
the correct Host/scheme and configure the exact public HTTPS origin. If trusting
Cloudflare headers, prevent direct origin access. Test redirects, cookies, OAuth,
uploads and canonical URLs through the actual proxy.

## Update and stop

```sh
./backup.sh
./restart.sh
./stop.sh
```

Restart updates from GitHub `main`, requires a clean source tree, then rebuilds.
Use `--no-pull` to rebuild an existing checkout without fetching source.
Stop preserves runtime data.

Compose mounts `./data` at `/data` for BadgerDB, auth, search and media. Keep it
and `.env` out of public source archives. See [recovery](operations.md) and the
[source management](source-control.md).

For native deployment, build frontend assets before `go build ./cmd/techpedia`,
export settings explicitly and supervise the binary with a service manager.
The native binary does not load `.env`.
