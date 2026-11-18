# Quick start

## Requirements

- Git and Docker Engine with Docker Compose v2 for the supported deployment path.
- A modern browser. Firefox is supported; no Chrome-only browser API is required.
- Free disk space for images, Go/Node build layers, and runtime data.

Clone `https://github.com/wavefnd/techpedia.git` on `main`.
Do not substitute a similarly named third-party repository.

## Configure a local installation

From the repository root:

```sh
cp .env.example .env
chmod 600 .env
```

Edit `.env` before starting. For local use, set:

```dotenv
TECHPEDIA_ENABLE_HTTPS=false
TECHPEDIA_PUBLIC_URL=http://localhost:8080
TECHPEDIA_HTTP_BIND=127.0.0.1
TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME=your_owner_name
TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD=your-own-long-random-password
TECHPEDIA_INTERFACE_AUTO_TRANSLATION=false
TECHPEDIA_MCP_ENABLED=false
```

Replace the sample password; do not use that literal value. For example,
`openssl rand -base64 48` generates a random password. Do not commit `.env`.

```sh
./start.sh
```

Open `http://localhost:8080`, sign in with the bootstrap owner account, then
configure [language editions](language-editions.md). An empty database does not
contain a ready-made encyclopedia or a ready-to-edit main page.

## First editorial setup

1. Open the language-edition section in the administrator panel.
2. Configure interface messages and navigation for an accepted edition.
3. Create and assign its administrator-controlled main page.
4. Activate the edition, then create an ordinary article.
5. Create another-language article and link both through Techdata when needed.

New regular accounts inherit the administrator-configured regular-user permission
policy; the fresh policy has no permissions. Grant `edit` deliberately before
expecting an account to edit or upload. Do not confuse an editor with an admin.

## Stop or rebuild

```sh
./stop.sh
./restart.sh --no-pull
```

Stopping preserves `data/`. The second command rebuilds the current checkout
without downloading source updates.

## Develop without Docker

Use the Go version in `go.mod` and Node 22 or newer for the frontend tools.
From the repository root:

```sh
npm ci --prefix web
npm ci --prefix docs
./tests/run.sh
./tests/server.sh
```

The dedicated helper binds to `127.0.0.1:18090` and keeps test DBs, logs and binaries
in `.local/test/`. It does not read the root `.env` or reuse `data/`. Explicit
test-only owner variables are described in the repository's
[test guide](https://github.com/wavefnd/techpedia/blob/main/tests/README.md).

A directly launched native Go process reads exported environment variables, not
`.env`. It defaults to `0.0.0.0:8080`; Compose's host-loopback binding does not
apply to that path. Prefer the test helper for development and configure native
production deployment deliberately.
