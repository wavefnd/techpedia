# Test workspace

Tests and local runtime data are separate from a deployed wiki.

| Location | Purpose |
| --- | --- |
| `internal/**/*_test.go` | Go package unit, storage, handler and parser tests |
| `web/tests` | Vue/component/service unit tests |
| `tests/integration` | Cross-system deployment and local-runtime checks |
| `tests/performance` | Browser/low-end audit tools |
| `benchmark` | Standalone disposable load-benchmark module |
| `.local/test` | Ignored local DB, logs, binary and temporary files |

Keep Go tests with their packages so private-package behavior remains testable.
Do not move unit tests into an artificial external package only to centralize paths.

## Run checks

Use the Go version in `go.mod`, Node 22 or newer, and Python 3 for the benchmark
tool tests. Install dependencies once:

```sh
npm ci --prefix web
npm ci --prefix docs
./tests/run.sh
```

`./tests/run.sh unit` runs frontend typecheck/tests/build followed by Go tests and
vet. `./tests/run.sh integration` checks scripts and cross-system fixtures without
starting the real wiki, Docker, or any network client. `all` runs both suites,
benchmark-tool tests and the documentation build. Temporary fixtures use
`.local/test/tmp` when invoked through this entry point.

Unit checks and native server builds share `.local/test/build.lock` so concurrent
commands cannot clear each other's frontend output while Go embeds it. A second
build stops instead of waiting indefinitely. The lock is released on normal
completion, failure or ordinary termination. After a forced kill, first confirm
no build is active, then remove only the empty lock with
`rmdir .local/test/build.lock`.

The complete suite does not implicitly run race or load tests. Run these explicitly
on suitable hardware:

```sh
go test -race ./internal/auth ./internal/storage ./internal/server ./internal/techmark
node tests/performance/low-end-audit.mjs http://127.0.0.1:18090/en/wiki/Example
```

## Native test wiki

```sh
./tests/server.sh
```

This builds frontend assets and a native binary, then runs in the foreground at
`http://127.0.0.1:18090`. Use Ctrl+C to stop. The log is `.local/test/server.log`;
the database/auth/media are under `.local/test/data`. The default build uses
`CGO_ENABLED=0`, matching the production binary build. Use `--build-only` to prepare
the binary, `--no-build` to reuse it, or `--check` to inspect safe paths/settings.
Choose another unprivileged port with `TECHPEDIA_TEST_PORT`.

The helper never reads the root `.env`, never copies or reuses `data/`, and does
not inherit production `TECHPEDIA_*` variables. It disables MCP, automatic external
translation and analytics. It does not start Docker or stop another listener.
An occupied port is an error, not permission to kill a process.

For a local owner account, explicitly set both `TECHPEDIA_TEST_OWNER_USERNAME` and
`TECHPEDIA_TEST_OWNER_PASSWORD` in your shell before starting. Use a test-only
random password, not a production credential. Without them the database has no
bootstrap owner. Existing test data persists between runs; changing environment
credentials does not reset an existing account or transfer ownership.

No reset command is provided. Review and back up test data before removing it.
Never treat the production database as a test fixture.
