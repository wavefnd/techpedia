# GitHub source transition

The public source destination is
[wavefnd/techpedia](https://github.com/wavefnd/techpedia), branch `main`.
GitLab `new` and `old` remain in GitLab. The owner archives GitLab manually;
deployment scripts never delete or archive those branches.

## Existing server

First receive the final preparation commit from GitLab using the existing update
process. Keep the same installation directory, `.env` and `data/`. Until the
GitHub repository is published, rebuild received code with:

```sh
./restart.sh --no-pull
```

After publication, `./restart.sh` updates from
`https://github.com/wavefnd/techpedia.git` and `main`. On the known legacy
GitLab origin it replaces that source URL, fetches main, and switches to main
without rewriting local legacy branches. Dirty trees, unexpected remotes, and
divergent main histories stop with an error. No reset/force checkout is performed.

This changes source only. It does not upload runtime data to GitHub or restore
the developer's test database.

## One initial public commit

Public history must start from a reviewed tracked snapshot, not a GitLab mirror.
After committing and checking the preparation in GitLab:

```sh
./scripts/export-public.sh /absolute/path/to/new-empty-public-directory
```

The exporter requires a clean source checkout and a nonexistent destination.
It archives tracked HEAD only, checks forbidden runtime/private paths, initializes
a separate `main` repository and creates one root commit. It never copies `.git`,
pushes, rewrites GitLab, or deletes local branches. Review the result, dependency
notices and publication rights before adding the GitHub remote and pushing main.
Do not publish old GitLab tags/branches by using `--mirror` or `--all`.

The exporter is a path guard, not a comprehensive secret scanner. Review tracked
source and asset rights separately; never assume ignored files prove a repository
contains no secrets.

## Documentation hosting

Enable GitHub Actions as the Pages source after publishing. The workflow deploys
only static English documentation. See [documentation build](documentation.md).
