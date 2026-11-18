# Repository Guidelines

This document defines the working rules for contributors and AI agents modifying TechPedia.
Treat it as the repository-local operating manual. If another instruction conflicts with this file, prefer the more specific task instruction from the user, but do not silently violate the architecture rules below.

## Project Identity

TechPedia is a topic-independent Go wiki engine distributed under the Blue Oak Model License 1.0.0 (SPDX: BlueOak-1.0.0). The current installation focuses on computer science and software engineering; this is an editorial content policy, not an engine constraint. General-purpose means a wiki, not a CMS, blog or social network.

The product goal is not to clone MediaWiki or Namuwiki. TechPedia should provide a Wikipedia-style multilingual document model, fast wiki navigation, readable technical articles, and an admin-friendly editing surface tailored for TechPedia's own content.

Primary goals:

- Stable multilingual wiki URLs based on the creation-time document name: `/ko/wiki/{name}` and `/en/wiki/{name}`
- Separate documents per language, connected by a language-independent Techdata item
- One engine and one user/auth system across languages
- Readable technical documentation pages with strong internal linking
- Simple deployability as a Go service, with static assets embedded into the binary

Non-goals:

- Reintroducing MediaWiki as a runtime dependency
- Building a general CMS, blog platform, or social network
- Adding a second frontend framework or replacing the approved Vue 3 stack without a proven blocker
- Splitting Korean and English into unrelated deployments
- Storing document content in scattered ad hoc files instead of the document store

## Architecture Principles

TechPedia is intentionally small and layered. Do not collapse the layers for short-term convenience.

Primary areas:

- `cmd/techpedia`: executable entry point
- `internal/config`: runtime configuration and environment handling
- `internal/document`: WID-based document model and revision/search value types
- `internal/techdata`: TID/PID items, statements, multilingual labels, and wiki-link invariants
- `internal/storage`: BadgerDB-backed document and Techdata persistence, indexes, and XML/JSON encoding
- `internal/techmark`: TechMark parsing, document-name URL normalization, and markup rules
- `internal/server`: HTTP routing, API handlers, system pages, and static asset embedding
- `web`: Vue 3, TypeScript, Vite, browser UI, and frontend source styles
- `internal/server/static/dist`: generated frontend assets embedded by the Go server
- `internal/auth`: authentication integration boundary
- `docs`: architecture and markup documentation

Design responsibilities:

- The server owns routes, HTTP behavior, and API response shape.
- The storage layer owns persistence, revision history, search data access, Techdata storage, and derived indexes.
- The Techdata layer owns language-independent identity, properties, statements, multilingual metadata, and WID mapping invariants; document content must not contain translation ownership fields.
- The TechMark layer owns markup parsing and document-name URL normalization rules. It should not query storage directly.
- Vue and browser code may enhance rendering and editing, but must not become the only place where core document rules exist.
- System pages should remain deterministic and available even when the document store is empty.

## Multilingual Document Rules

TechPedia uses language-prefixed URLs as the canonical public shape.

Canonical examples:

- Korean: `/ko/wiki/운영체제`
- English: `/en/wiki/Operating_System`
- Document API: `/api/pages/{locale}/{name}`
- Revision API: `/api/pages/{locale}/{name}/revisions`; public labels use per-document `r1`, `r2`, ... values and never expose storage keys.

Document identity is defined by:

- `wid`: immutable wiki document identifier such as `W135`
- `locale`: lower-case IANA language subtag without script, region, variant, extension, or private-use suffix; `ko` and `en` are examples, not a closed enum or automatically activated persistent editions
- `name`: user-selected document name that also determines the canonical URL

Language-independent identity is defined separately by:

- `tid`: immutable Techdata item identifier such as `T42`
- `Item.WikiPages`: one optional `WID` per supported locale
- reverse index: `wid -> tid`
- `pid`: immutable Techdata property identifier used by typed statements

Rules:

- Do not create `/wiki/{name}/en`, `/{name}/en`, query-string language switching, or `index.php` style public URLs.
- The document name entered during creation determines both the display name and canonical URL.
- Do not add a separate user-editable `slug`, URL name, or path field.
- TechMark `#` headings do not determine the document name or URL.
- Korean and English pages may have different names and source text while belonging to the same Techdata item.
- Language switching must use Techdata wiki links and available WIDs, not guessed title suffixes.
- Missing translations should be treated as creatable linked pages, not as errors.
- System pages must exist in both Korean and English where practical.
- New language editions are stored in the separate BadgerDB edition registry with `pending`, `approved`, or `active` status.
- Editors may propose one registered base IANA language code. Administrators accept or reject every proposal; only an accepted edition may be configured and activated.
- General article creation and editing must remain blocked until the language edition is active and has a main page.
- HTML `lang`, `Content-Language`, URL locale, Techdata locale keys, and SEO language metadata must use the same lower-case language code.
- Do not create script or region editions. The engine must not hardcode language-merging, aliasing, or acceptance policy; administrators decide whether each proposed base language becomes an edition.
- The writing system inferred from IANA and CLDR metadata is used for display direction and automatic-translation validation, not as a character filter for document titles or content. Technical documents must remain able to contain arbitrary Unicode text, code, symbols, and foreign names.
- New document models, XML blobs, and save requests must not add or depend on `group`, `slug`, `tid`, or translation fields inside the document.
- Legacy `group` data may only be used by an optional one-time import tool; do not add a permanent compatibility reader to the target runtime.
- Document-name changes are explicit move operations. They preserve `WID`, revisions, and Techdata membership and create a redirect from the old name.
- WIDs, TIDs, and PIDs are not exposed in public UI or public DTOs; they may appear only on authenticated administrator screens and APIs.

## Document And Markup Policy

TechMark is the source markup for TechPedia documents.

Keep the syntax easy to write:

- Markdown-like headings and emphasis are acceptable.
- `[[문서]]` internal links are first-class wiki links.
- Custom tags such as `<wiki-infobox>` and `<wiki-alert>` are allowed for richer technical documents.

Rules:

- Do not add complex MediaWiki template syntax.
- Do not make parsing depend on browser-only behavior for security or persistence correctness.
- Preserve source text when saving revisions.
- Generated HTML must be safe to render and predictable for the frontend.
- Update `docs/techmark.md` when user-facing markup behavior changes.

## Storage Rules

The current document store is BadgerDB with XML blob encoding.

Rules:

- Keep `internal/storage/store.go` as the storage contract.
- Keep document and Techdata behavior behind explicit storage interfaces before wiring handlers.
- Maintain revision history when saving document updates.
- Search should stay locale-aware.
- Page lookup should use `wiki:v1:name:{locale}:{normalizedName} -> wid`.
- Translation lookup should use `techdata:v1:item:{tid}` and the `techdata:v1:wiki:{wid}` reverse index.
- Techdata item and reverse wiki mappings must be changed in one BadgerDB transaction.
- A `WID` may belong to at most one Techdata item, and an item may contain at most one `WID` per locale.
- Techdata items use versioned JSON values; document content and revisions remain XML blobs.
- Do not introduce a separate SQL database for documents unless the architecture document is updated first.
- Do not store production secrets, database folders, or generated runtime data in git.

## API And Admin Rules

Write operations use account sessions and explicit permissions.

The optional `/mcp` endpoint is disabled by default and requires an administrator-approved OAuth bearer token. Registration, consent and revocation belong only in `/admin/connections`; every request and refresh must verify current admin/owner permissions and the same immutable account identity. `wiki:read` reads public source/history; separate `wiki:propose` consent allows only immutable create/edit/language-link proposals. A current admin/owner cookie session must review the exact diff/link and explicitly decide in `/admin/proposals`; application, snapshot checks and source-free audit commit transactionally. Existing read grants are never upgraded automatically. No delete, unlink, merge, approve or commit MCP tool is permitted. Document deletion is owner-only. Keep integrations behind `internal/wiki` and explicit storage contracts. Tool annotations, ChatGPT confirmations and model `confirmed` arguments are not external write authorization. The large bot platform is deferred, not removed; report routine implementation progress in the conversation, not issues.

Rules:

- Keep read routes usable without authentication.
- New registered accounts inherit the configured regular-user permission policy (empty on a fresh installation); Techdata and user management require `techdata` and `admin` respectively.
- Store sessions in `HttpOnly`, `SameSite=Lax` cookies and store only token hashes server-side.
- Current password credentials use Argon2id. The passwordless target is Google Authenticator-compatible TOTP with encrypted secrets and recovery codes.
- Return clear HTTP status codes and JSON errors for API failures.
- Keep API routes stable when possible; frontend code and migration scripts may depend on them.
- Administrator features should improve editorial control: language linking, revision visibility, missing translation creation, system page management, and document metadata.
- Public API DTOs must not mirror storage models blindly.
- Keep raw WIDs, TIDs, storage keys, search scores, and opaque revision IDs out of normal reading responses unless a specific action requires them.
- Techdata wiki linking, unlinking, property changes, and statement changes belong to protected administrator endpoints, not editable document fields.

## Frontend Rules

The target frontend is Vue 3, TypeScript, Vite, Vue Router, and the existing `wiki-*` Web Components. Source code lives under `web`; Vite output is embedded by the Go server.

Rules:

- Do not add React, Next.js, Nuxt, Svelte, or another UI framework alongside Vue.
- Do not add Pinia, Axios, or a UI component framework until a concrete need is documented.
- Use browser `fetch` for the initial API layer and TypeScript types for public DTOs.
- Treat `internal/server/static/dist` as generated output; do not edit it manually.
- Configure Vue to preserve `wiki-*` custom elements used by TechMark.
- Keep the reading experience document-first, not dashboard-first.
- Use language switch controls that map to each page's actual document-name URL.
- Missing internal links should be visually distinct and lead toward document creation.
- Avoid card nesting and overly decorative layouts; technical pages must remain readable.
- Maintain mobile readability for article pages, editor panels, search, history, and language controls.
- Static assets should remain embeddable by the Go server.
- Do not display raw revision IDs, WIDs, TIDs, translation keys, search scores, engine state, or local/API implementation details in the normal article UI.
- Do not show a slug or URL-name input. Keep document-name moves and translation mapping out of the basic article editing form; expose them through deliberate administrator tools.

## System Pages

Built-in system pages let the wiki work before user content exists.

Current system page examples:

- Korean `대문`, English `Main Page`
- Korean `최근 변경`, English `Recent Changes`
- Korean `임의 문서`, English `Random Page`
- Korean `도움말`, English `Help`
- Korean `저작권`, English `Copyright`
- localized technical category pages

Rules:

- System pages should be available per locale.
- System pages should participate in search when helpful.
- User-created documents may coexist with system pages, but route behavior must stay predictable.
- Do not add `/ko/wiki/main` or `/en/wiki/main` compatibility routes; TechPedia is not yet a production service and uses clean canonical names.
- Update README and architecture docs when system page behavior changes.

## Coding Style

Use idiomatic Go.

Rules:

- Format Go code with `gofmt`.
- Keep package names short and lower-case.
- Prefer small interfaces at package boundaries.
- Pass `context.Context` through storage and request-driven operations.
- Return explicit errors instead of panics for request and storage failures.
- Keep handlers readable; extract helpers when routing logic starts hiding behavior.
- Avoid global mutable state except for embedded static assets or immutable lookup tables.
- Keep comments useful and sparse. Explain non-obvious rules, not ordinary syntax.

JavaScript and CSS rules:

- Keep dependencies limited to the approved Vue/Vite stack and CodeMirror 6 editor packages unless another dependency is clearly justified.
- Prefer explicit DOM state updates over hidden global behavior.
- Keep CSS organized around actual UI surfaces: article, navigation, editor, history, search, and system pages.
- Do not use minified or generated frontend files as source files.

## Testing And Verification

Run focused tests when changing behavior.

Standard local verification:

```bash
./tests/go.sh test ./...
```

Useful checks:

```bash
./tests/go.sh test ./internal/techmark
./tests/go.sh test ./internal/storage
./tests/go.sh test ./internal/server
go run ./cmd/techpedia

cd web
npm run typecheck
npm run test
npm run build
```

Rules:

- Add or update tests for parser, document-name normalization, storage, route, and API behavior changes.
- Multilingual behavior needs tests for `ko`, `en`, a newly activated edition, rejected script/region tags, and independently reviewed related-language codes when practical.
- Storage behavior should be tested through the public storage interface.
- Techdata tests must cover forward/reverse wiki index consistency, duplicate locale conflicts, typed values, and transaction rollback.
- Vue changes should pass type checking and focused component tests where behavior is non-trivial.
- Do not require Docker for normal unit tests.
- Store all Go test source files under `tests/go/`, mirroring their original repository paths. Run them with `./tests/go.sh`, which uses a temporary Go overlay to preserve package-private access without copying tests into production directories or exporting internals. Plain `go test ./...` does not load these tests and is not a complete verification command. Keep Vue unit tests in `web/tests`, cross-system tests in `tests/integration`, performance/browser audit tools in `tests/performance`, and local test state under ignored `.local/test`.
- Use `./tests/run.sh` for the aggregate suite and `./tests/server.sh` for a loopback-only native test installation. Never load production `.env` or reuse production `data/` in test helpers.
- If tests cannot run in the current environment, state that clearly in the final response.

## Documentation Rules

Docs are part of the product design.

Report implementation progress and verification results in the current conversation,
not by automatically creating or updating issues. Create or update a GitHub issue
only when explicitly requested by the user. Public issue and pull-request text
must be in English.

- Check existing issues before creating a requested issue. Include evidence, confirmation level, scope, and completion criteria; do not report an unverified suspicion as a reproduced bug.
- Keep documentation for confirmed architecture, API contracts, markup, operational procedures, and historical measurements, not new roadmap or TODO lists.
- Link relevant existing issues from design documents when useful. An issue or future-version label does not authorize implementation or an engine-version change.
- Never publish credentials, private runtime data, or document/user personal information in an issue.

The backlog entry point is https://github.com/wavefnd/techpedia/issues.

Update documentation when changing:

- public routes
- API request or response shape
- document identity rules
- TechMark syntax
- storage model
- deployment commands
- admin/security behavior
- visible multilingual behavior

Important files:

- `README.md`: practical run/API surface
- `docs/arch.md`: architecture and product direction
- `docs/techdata.md`: Techdata identity and language links
- `docs/techmark.md`: markup behavior

Repository documentation and script messages use English. The administrator UI remains Korean and public editions remain multilingual. Keep identifiers such as route paths, environment variables, package names, and commands exact. Build the isolated VitePress documentation site with `npm ci --prefix docs && npm run build --prefix docs`; it must not enter the engine's reading bundle.

## Git And Generated Files

Engine versions use `vMAJOR.MINOR.PATCH`. PATCH is for bug fixes and small compatible adjustments; MINOR is for new features and MAJOR for breaking changes. The existing `1.10` is normalized to `1.10.0` by the owner's approval, not a new feature release. Preserve historical tags such as `v1.10-3`. The owner has authorized a PATCH increment when explicitly requesting a push of fixes; do not increment while merely implementing or testing, or increment again when retrying a failed push of the same release. MAJOR and MINOR still require an explicit version declaration.

Rules:

- Do not commit `.DS_Store`, IDE caches, local data directories, or production secrets.
- Keep `go.mod` and `go.sum` consistent with actual imports.
- Avoid unrelated formatting churn.
- Do not rewrite user changes unless explicitly requested.
- Prefer small, reviewable commits grouped by feature or fix.

## Deployment Expectations

TechPedia should remain simple to deploy.

Rules:

- During the migration, the current Go server should remain runnable with `go run ./cmd/techpedia`.
- After the Vue switch, production assets must be built with `npm ci && npm run build` before the Go binary is built.
- Docker support should stay optional but production-ready.
- Production Docker builds should use a Node frontend stage and a Go backend stage; the final image should not require Node.js.
- Environment variables should be documented when added.
- Runtime data paths should be configurable and excluded from git.
- Public URLs should stay clean and language-prefixed.
