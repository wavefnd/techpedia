# Write and edit

Create a document by entering its name and TechMark source. The name becomes its
display title and canonical URL; there is no separate slug field. A source
heading does not rename a page.

## Save a revision

Sign in with the required permission, open the editor, and review the preview.
Describe the change, then save against the revision you opened. If another edit
wins first, reload and reconcile the conflict; do not overwrite a newer revision
by guessing its revision number. Published source is retained in revision history.

The editor provides image and reusable-box insertion. [TechMark](techmark.md)
defines the source syntax. Preview is useful but the Go API remains responsible
for permissions, protected syntax, active editions, and concurrency.

## History, moves, and redirects

History lists per-document labels such as `r1`, `r2`, and diffs. Restoring old
content creates a new revision and must still satisfy the current edit policy.

Use the administrator's explicit move operation to rename a document. Its stable
identity, history, and Techdata membership remain; the old name becomes a redirect.
Administrators can also create aliases, for example `BSD` pointing to
`Berkeley Software Distribution`. Use the full canonical name for the document.

## Permissions and destructive actions

An `editors` policy permits users with `edit`; an `admins` policy requires an
administrator. New accounts inherit the configured regular-user permission policy,
which is empty in a fresh database. Admins assign permissions deliberately.

Document deletion is owner-only. History reset is an owner-only privacy operation,
not an ordinary revert: it replaces the current safe content with a new r1 while
preserving identity and creation date, removes old copies and related proposals,
and invalidates derived caches. Back up and review before using it.

Do not put secrets in documents, edit summaries, drafts, or proposals. An ordinary
edit does not erase previous revisions.
