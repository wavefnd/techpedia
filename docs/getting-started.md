# Quick start

## Requirements

- Git and Docker Engine with Docker Compose v2 for the supported deployment path.
- A modern browser. Firefox is supported; no Chrome-only browser API is required.
- Free disk space for images, Go/Node build layers, and runtime data.

Clone `https://github.com/wavefnd/techpedia.git` on `main` after publication.
During preparation, use the approved GitLab checkout and rebuild with
`./restart.sh --no-pull`. Do not substitute a similarly named third-party repository.

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
npm run build --prefix web
go test ./...
TECHPEDIA_ADDR=127.0.0.1:8080 go run ./cmd/techpedia
```

The native Go process consumes exported environment variables; it does not load
`.env` itself. Export bootstrap credentials before launching it. Native Go defaults
to `0.0.0.0:8080` unless `TECHPEDIA_ADDR` is set; Compose's host-loopback binding
does not apply to this path. Do not expose a development listener publicly.
