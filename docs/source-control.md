# Source management

The authoritative repository is
[wavefnd/techpedia](https://github.com/wavefnd/techpedia), branch `main`.
Use its GitHub issues and pull requests for development. Release versions are
maintainer decisions; ordinary work must not rewrite published history.

## Fresh checkout

```sh
git clone --branch main https://github.com/wavefnd/techpedia.git
cd techpedia
```

Use `codex/` branches for agent implementation work when a separate branch is
needed. Never push credentials, runtime data, backups, local test files or build
output. Keep `.env` private and choose an appropriate public Git author email.

## Server updates

`./restart.sh` checks for a clean `main` checkout and the approved GitHub origin,
fetches `origin/main`, performs a fast-forward merge, then rebuilds and starts.
It does not change origin, switch branches, force-reset divergent history or
overwrite runtime data. `--no-pull` rebuilds the current checkout only.

If source history has intentionally been rewritten, a normal fast-forward may
fail. Preserve local changes and runtime data, then obtain a fresh checkout or
follow a deliberate maintainer-reviewed recovery plan. Do not delete `.env` or
`data/` to resolve a source-history conflict.

## Source releases

For a source-only archive, inspect the tracked tree and use `git archive` for a
reviewed commit. Do not include a working directory or `.git`, and do not mirror
unpublished branches. The engine license does not license an installation's wiki
content or user data. Retain third-party notices.

`move.sh` is a private operator transfer including local Git metadata, not a public
release archive. It excludes runtime data, environment files and local test output.

## Local testing

The `tests/` directory holds integration tests and local execution helpers.
Go test sources are stored separately under `tests/go/` and executed in their
original package context by `./tests/go.sh`. Vue unit tests remain in `web/tests`.
Test DBs, logs, binaries and temporary files belong to ignored `.local/test/`.
Never load production `.env` or reuse `data/` in the local test server.

See [deployment](deployment.md), [operations](operations.md) and the repository's
[test guide](https://github.com/wavefnd/techpedia/blob/main/tests/README.md).
