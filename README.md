# TechPedia

A multilingual, topic-independent wiki engine maintained by Wave Foundation.
Go and BadgerDB serve revisioned TechMark documents with a Vue 3 frontend embedded
in the executable. It is a wiki, not a CMS, blog, or social network.

Original engine source is licensed under the [Blue Oak Model License 1.0.0](LICENSE),
SPDX `BlueOak-1.0.0`. Copyright 2026 Wave Foundation.
Third-party dependencies and fonts retain their own terms.

## Documentation

The English guide covers installation, writing, administration, recovery, security,
API boundaries and architecture:

- [Documentation source](docs/index.md)
- [Quick start](docs/getting-started.md)
- [Server deployment](docs/deployment.md)
- [Configuration](docs/configuration.md)
- [Operations and backups](docs/operations.md)
- [TechMark](docs/techmark.md)
- [Architecture](docs/arch.md)

After GitHub Pages is enabled, the guide is hosted at
[wavefnd.github.io/techpedia](https://wavefnd.github.io/techpedia/).
The source destination is [wavefnd/techpedia](https://github.com/wavefnd/techpedia),
branch `main`. These addresses become available after publication; preparation
is first delivered through the existing private GitLab repository.

## Run locally

Install Git, Docker Engine and Compose v2, then:

```sh
cp .env.example .env
chmod 600 .env
# Edit .env: choose a real owner username and a long random initial password.
./start.sh
```

The example uses `http://localhost:8080` with a loopback host binding.
Configure accepted language editions, interface messages and a main page in the
administrator panel before ordinary editing. A fresh installation has no imported
articles, runtime database or pre-approved language editions.

The owner identity is immutable. Newly registered regular users inherit the
configured permission policy, empty by default. Public reading is anonymous;
write permissions are verified in Go. The administrator UI remains Korean;
public messages and navigation remain configurable per language edition.

## Main capabilities

- Name-based `/{locale}/wiki/{name}` URLs, revision history, diffs and explicit moves.
- Independent language documents connected through central Techdata identity.
- TechMark links, infoboxes, navigation boxes, media carousels and reusable boxes.
- Administrator-managed editions, permissions, redirects, discussions and audit.
- Optional administrator-only OAuth/MCP reads and proposals with human diff review;
  no direct publish or document deletion through MCP.
- Self-hosted fonts and server-rendered articles with lazy optional tools.

## Development

Use the Go version in `go.mod` and Node 22 or newer. Build frontend assets before Go:

```sh
npm ci --prefix web
npm run typecheck --prefix web
npm run test --prefix web
npm run build --prefix web
go test ./...
go vet ./...
TECHPEDIA_ADDR=127.0.0.1:8080 go run ./cmd/techpedia
```

Native Go reads exported environment variables, not `.env`. Docker is optional
for unit tests. Generated frontend assets, dependencies, databases and credentials
are excluded from Git.

Documentation is an isolated VitePress package, not a dependency of the reader:

```sh
npm ci --prefix docs
npm run dev --prefix docs
npm run build --prefix docs
```

## Updates and distribution

`start.sh` builds current source; `restart.sh` updates from GitHub `main` and
rebuilds; `--no-pull` rebuilds without a source fetch. Use that flag while GitHub
publication is pending. Scripts preserve existing `.env` and `data/`.
See [the transition guide](docs/source-migration.md). GitLab legacy branches are
not rewritten or deleted, and archiving is performed by the owner.

Public export uses a separate reviewed tracked snapshot with one initial main
commit. `move.sh` includes private Git history and is **not** a public exporter.
Never publish wiki content, images, user data, backups, or secrets as engine source.

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Report vulnerabilities privately.
Engine versions use `vMAJOR.MINOR.PATCH`; maintainers decide releases.
