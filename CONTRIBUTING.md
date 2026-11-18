# Contributing

TechPedia is a Go/BadgerDB wiki with a Vue 3/TypeScript frontend. Wiki content,
accounts, images, and installation settings are separate from the engine source.

Discuss ordinary bugs and focused improvements in the repository's issue tracker.
Include reproduction steps, expected and actual behavior, and relevant versions.
Never attach credentials, private source documents, databases, or a real `.env`.
Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

Keep changes small, link the issue, and update tests and the relevant English
guide. Do not change the engine version without a maintainer's release decision.
Preserve the Korean administrator interface and configurable multilingual public
interface. Read [AGENTS.md](AGENTS.md) for architecture and permission boundaries.

## Local checks

Use the Go version in `go.mod` and Node 22 or newer. Build web assets before Go:

```sh
npm ci --prefix web
npm run typecheck --prefix web
npm run test --prefix web
npm run build --prefix web
go test ./...
go vet ./...
npm ci --prefix docs
npm run build --prefix docs
./tests/run.sh integration
git diff --check
```

Format Go with `gofmt`. Never commit generated `dist`, `node_modules`, or runtime
data. Server permission checks are mandatory; browser route guards are not a
security boundary. Consider mobile, RTL, arbitrary Unicode, and the initial
article bundle when proposing a change.

Cross-system tests and local execution helpers live in `tests/`; their runtime
data stays in ignored `.local/test/`. Keep Go tests beside their packages and
Vue unit tests in `web/tests`. See [tests/README.md](tests/README.md).

## Rights

You must have the right to submit your contribution. Unless agreed otherwise,
accepted original contributions use the [Blue Oak Model License 1.0.0](LICENSE); copyright
assignment is not required. Preserve upstream copyright, license, NOTICE, and
modification notices for third-party material. Do not relabel it as Blue Oak.
