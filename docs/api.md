# API boundaries

The Go router owns HTTP behavior; public DTOs are not raw storage models.
Endpoint implementations and tests remain authoritative for detailed fields.

## Public reads

| Route | Purpose |
| --- | --- |
| `GET /api/health` | Health check |
| `GET /api/config` | Public engine settings |
| `GET /api/languages` | Public editions |
| `GET /api/language-catalog` | Registered base-language search |
| `GET /api/pages/{locale}/{name}` | Article |
| `GET /api/pages/{locale}/{name}/revisions` | Revision summaries |
| `GET /api/pages/{locale}/{name}/revisions/{revision}` | One revision |
| `GET /api/pages/{locale}/{name}/diff` | Revision comparison |
| `GET /api/pages/{locale}/{name}/backlinks` | Incoming links |
| `GET /api/search` | Locale-aware search |
| `GET /api/graph` | Link metadata graph |
| `GET /media/files/{name}/{variant}` | Uploaded media variant |

Public read responses exclude WIDs, TIDs, PIDs, storage keys and opaque revision
IDs. Names are normalized by TechMark rules, not generated slugs. History responses
are not a source of writable storage revision tokens.

## Authentication and writes

`POST /api/auth/signup` and `POST /api/auth/login` require JSON and same-origin
browser requests; `POST /api/auth/logout` checks origin. Credential-work saturation
returns 429 with Retry-After. Session cookies are HttpOnly/SameSite=Lax.

Document writes require sessions and current edit/admin policy. Save and restore
compare the exact revision observed for authorization. Stale/future bases return
409 rather than overwriting a concurrent policy/content change. Initial creation
uses the empty base; clients must not guess the next revision.

Protected `/api/admin/*` routes manage editions, users, redirects, Techdata,
boxes, source proposals, support and audit. Owner-only operations include document
deletion, privacy history reset and unused-item deletion with explicit confirmation.
Do not infer write authorization from a client-side route or hidden button.

## MCP proposals

MCP read/propose tools use an approved administrator OAuth token, not an ordinary
reader cookie. Administrator review uses a separate cookie session and exact
proposal digest.

- `GET /api/admin/mcp/proposals`: source-free summaries.
- `GET /api/admin/mcp/proposals/{id}`: detail and review digest.
- `GET /api/admin/mcp/proposals/{id}/diff`: paginated diff.
- `POST /api/admin/mcp/proposals/{id}/decide`: explicit decision.
- `POST /api/admin/mcp/proposals/decide-batch`: 1–100 explicit decisions.

Batch application is per-proposal, not one all-or-nothing transaction. Inspect
individual results. Never retry an uncertain write automatically after response
loss. See [integrations](integrations.md) for the consent/review boundary.
