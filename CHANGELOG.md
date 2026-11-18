# TechPedia changelog

Versions use `vMAJOR.MINOR.PATCH`. PATCH covers fixes and small compatible changes.
Maintainers decide releases; implementation alone does not increment a version.
Historical two-part tags and tags such as `v1.10-3` remain unchanged in GitLab.
This English history summarizes the original development record.

## v1.10.12

- Prepare original engine source under the Blue Oak Model License 1.0.0, copyright Wave Foundation.
- Replace development roadmaps with English VitePress user/operator guides and
  GitHub Pages build/deployment workflows.
- Set server source updates to GitHub wavefnd/techpedia main, preserving runtime
  data; keep GitLab legacy history separate.
- Bound credential work, reject cross-site authentication, index closing-tag
  lookups, budget image output and bind saves/restores to authorized revisions.
- Add focused regression tests and distribution notices.
- Patch DOMPurify/Vite security advisories and use the native Docker build
  architecture when no explicit target architecture is supplied.

## v1.10.11

- Default document graph includes all metadata and avoids dense PageRank work;
  browser batches, Worker buffers and adaptive frequency reduce overhead.
- Add multilingual independent reusable boxes and editor insertion.
- Display multi-image blocks as horizontal carousels and clear floats at headings.
- Use full-response HTML ETags, bootstrap edition UI, correct descriptions/sitemap
  dates and reduce redundant code-highlight spans.

## v1.10.10

- Submit exact source-range replacements instead of entire documents.
- Read and replace pending proposals through the same authorized connection.
- Superseded proposals cannot be approved; administrators review the new diff.

## v1.10.9

- Use Worker spring/repulsion/damping/collision graph physics without forced
  cluster placement or color categories.
- Reduce calculation frequency at stability, pause hidden tabs and preserve state
  across explicit pause/resume.
- Correct node size units and text contrast.

## v1.10.4

- Fix image-picker grid placement and clipped bottom controls.
- Separate scrolling lists/forms from persistent actions; preserve upload validation.

## v1.10.3

- Accept comma-separated uploaded image filenames with order/duplicates preserved.
- Share alt/caption/layout and retain individual enlargement, responsive variants,
  lazy loading and infobox support. The original vertical layout was later
  replaced by the v1.10.11 carousel.
- Reject invalid/empty entries as a whole block; support escaped filename commas.

## v1.10.2

- Render uploaded and HTTPS images inside infoboxes, keeping row/image order,
  alt text, captions, attribution and enlargement.

## v1.10.1

- Add derived summary/search/backlink indexes and resumable index migration.
- Bound render/graph caches and batch aggregate statistics transactionally.
- Parallelize route preparation without bypassing permissions; preserve reader DOM.
- Reduce offscreen layout/link/TOC work and fix stale article/history responses.

## Earlier security and integration preparation

- Update Go to 1.26.8, x/image to 0.45.0 and x/text to 0.41.0, plus browser
  dependency security patches. Existing binaries still require rebuilding.
- Add opt-in administrator OAuth/PKCE MCP read integration and shared Go services.
- Fix stale administrator responses and improve audit labels/filtering.
- Keep internal identifiers and account metadata out of public responses.

## v1.10

- Owner-only privacy history reset retaining current safe content and identity.
- Owner-only deletion of unused Techdata items with complete reference checks.
- Address-only support QR codes, lazy generation and no payment execution.
- Replace hardcoded naming rules with title/Techdata-derived candidate indexes,
  exact collision handling, character N-grams and conservative fuzzy matching.
  Large autonomous bot-platform work remains deferred.

## v1.9 — 2026-07-20

- Add dedicated edition search and missing-document creation navigation.
- Correct canonical self-redirect and API/locale routing collisions.
- Add horizontal rules, regular-user permission policy, profile editing and
  public changelog; improve portal rendering/crawler behavior.

## v1.8 — 2026-07-17

- Improve low-end reading, SSR and crawler metadata.
- Add graph viewing, global uploaded media/variants and image enlargement.
- Preserve technical punctuation/case in document names and internal links.

## v1.7 — 2026-07-17

- Stabilize footnotes, code-fence language handling, parser output and descriptions.
- Cache search/render work, isolate administrator assets and count portal documents.
- Add in-reader administrator language-link management and atomic duplicate-item
  detection without overwriting existing labels.

## v1.6 — 2026-07-16

- Add revision-based optimistic writes, server drafts, history diffs and restores.
- Add change/contribution streams, trash recovery and audit.
- Add inline wiki-footnotes, safe code anchors, navboxes, trees and flows.
- Add collapsible sections, sticky navigation and server-rendered indexing signals.
- Add country filtering, Techdata link audit and optional support-address pages.

## v1.5 — 2026-07-15

- Persist WIDs across moves/history and derive language switching through Techdata.
- Add backlinks, missing-link reports and transactional translated-page creation.
- Derive naming candidates from Techdata instead of user-authored rule CRUD.
- Add server information, explicit location, backup/restore and portal metadata.

## v1.4 — 2026-07-15

- Add internal link crawling and administrator-reviewed naming suggestions.
- Preserve revisions during accepted moves and reject stale suggestions.
- Add top/bottom article navigation with reduced-motion/mobile handling.

## v1.3 — 2026-07-15

- Add administrator homepage panels, aggregate views and light/dark themes.
- Replace external comments with account-integrated document discussions.
- Preserve discussion/history/views on moves and fix missing-link creation names.

## v1.2 — 2026-07-15

- Add inline code, safe tables, Chroma fenced-code highlighting and system tags.
- Add hierarchical TOC, dedicated mobile reader and scrollable tables.
- Apply multilingual font stacks; retire inline wiki-toc.

## v1.1 — 2026-07-14

- Add fourth-level headings, list handling, document edit policies, regular-user
  permissions and document-local public revision labels.
- Expose engine version and document the original two-part version policy.

## v1.0 — 2026-07-14

- Establish Go/BadgerDB XML documents, Vue/TypeScript/Vite frontend and CodeMirror.
- Introduce name-based multilingual URLs and central Techdata identity.
- Add Argon2id accounts, revocable sessions, administrator tools, search and SEO.
- Add multi-stage Docker deployment and Caddy HTTPS scripts.
