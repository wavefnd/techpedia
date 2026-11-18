# Build and publish this documentation

The documentation is a separate VitePress site. It is not part of the embedded
wiki frontend and does not add JavaScript to ordinary article pages.

## Local commands

```sh
npm ci --prefix docs
npm run dev --prefix docs
npm run build --prefix docs
npm run preview --prefix docs
```

The local development and preview servers bind to loopback. Generated files live
in `docs/.vitepress/dist`; cache, generated output, and `docs/node_modules` must not
be committed. Production builds check internal links.

## GitHub Pages

The Pages workflow builds this site from `main` and deploys its static output.
Pull requests only build and check; they do not publish Pages or deploy the engine.
In the public repository, select **Settings → Pages → Source → GitHub Actions**.
Pages publishes this repository's documentation, not a private staging checkout.

The configuration derives a project site's base path from `GITHUB_REPOSITORY`.
For a project at `owner/repo`, assets use `/repo/`; an `owner.github.io` repository
uses `/`. For a custom domain, set the workflow's `DOCS_BASE` repository variable
to `/` and configure the domain and DNS through GitHub Pages separately.

Test a subpath build before publication:

```sh
DOCS_BASE=/techpedia/ npm run build --prefix docs
DOCS_BASE=/techpedia/ npm run preview --prefix docs
```

Pages hosts only this static guide. It does not run Go, BadgerDB, the administrator
panel, OAuth, or the wiki itself. Deploy the engine to a server separately.

## Writing

Use English Markdown with fenced code examples, task-oriented headings, and
relative Markdown links. Document implemented behavior and real limitations.
Keep development tasks in the issue tracker instead of adding roadmap pages here.
Never include an actual `.env`, private document, API token, database, or a
vulnerability exploit against an unpatched public installation.
