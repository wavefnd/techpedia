# Architecture

TechPedia is a topic-independent multilingual Go wiki engine under the Blue Oak Model License 1.0.0.
An installation's editorial focus is not an engine constraint. It is not a CMS,
blog, or social network. A single Go service embeds static Vue 3 assets; BadgerDB
stores XML document/revision blobs and versioned JSON operational/Techdata records.

## Layers

| Boundary | Responsibility |
| --- | --- |
| `cmd/techpedia` | Startup and wiring |
| `internal/config` | Environment and runtime paths |
| `internal/auth` | Credentials, sessions, permissions, immutable owner |
| `internal/document` | Document identity and revisions |
| `internal/techdata` | Items, typed statements, multilingual links |
| `internal/storage` | Transactions, blobs, indexes, history |
| `internal/techmark` | Safe markup and name normalization, no storage queries |
| `internal/wiki` | Shared read/proposal integration service |
| `internal/server` | HTTP, public DTOs, system pages, embedded assets |
| `web` | Vue/TypeScript reader, editor, Korean administrator interface |
| `docs` | Separate VitePress documentation, never an engine dependency |

## Identity and transactions

Canonical URLs are `/{locale}/wiki/{name}`. Entered names determine titles and
URLs; no slug input exists. A WID survives moves. Names use
`wiki:v1:name:{locale}:{normalizedName}`; revisions have document-local labels.

Techdata owns translation identity. An item maps at most one WID per edition;
`techdata:v1:wiki:{wid}` is the reverse index. Forward/reverse mappings change in
one transaction. Article XML has no group, slug, TID, or translation ownership.
Internal identifiers are restricted to authenticated administrator surfaces.

A write authorizes an observed document and validates that same base revision.
Atomic storage comparison is the final write boundary. Restoration creates a new
revision. Moves, deletion, recovery, privacy resets and box updates invalidate
the appropriate derived search/graph/render data.

## Editions and system pages

Edition codes are registered lowercase base IANA language subtags. Administrators
accept proposals, configure messages/navigation and a main page, then activate
editing. No language-merging or writing-system policy is hardcoded. Arbitrary
Unicode remains valid content; direction/font metadata is not a character filter.
Fresh persistent stores do not seed accepted language editions automatically.

System routing remains deterministic with an empty database. Operational tags
require an administrator and administrator edit policy; homepage panels also
require the configured main page.

## HTTP and security

Go supplies meaningful article HTML before JavaScript. Public DTOs exclude
storage IDs, opaque revision keys, account internals, and ranking/debug metadata.
Vue enhances existing article DOM. Editor/admin/graph/QR code is lazy-loaded.

Sessions use hashed server-side tokens and HttpOnly/SameSite=Lax cookies.
Argon2id work is budgeted; authentication endpoints enforce origin and JSON
content type. Go rechecks every permission rather than trusting route guards.

MCP is disabled by default and uses administrator OAuth read/propose consent.
Immutable proposals require exact diff/link review in an administrator cookie
session; there is no MCP delete, unlink, merge, approve, or direct-commit tool.
Snapshot checks, application and source-free audit commit atomically.

## Deployment

Runtime paths are configurable and excluded from Git. Docker uses Node frontend,
Go build, and final Alpine stages without Node. The documentation site is a
separate static build; GitHub Pages cannot run Go, OAuth, or BadgerDB.

Source updates use the approved GitHub origin and a clean `main` checkout with
fast-forward-only merges. Local test execution is separate: `tests/` owns
integration/runtime helpers, while ignored `.local/test/` holds test state.
The native test helper binds only to loopback and does not load production `.env`
or reuse `data/`. Package-level Go and Vue unit tests stay beside their code.

See [deployment](deployment.md), [configuration](configuration.md),
[source management](source-control.md), [API boundaries](api.md), and
[performance](performance.md).
