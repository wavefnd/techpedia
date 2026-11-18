# ChatGPT and MCP

The optional `/mcp` endpoint is disabled by default. It is an administrator
integration, not a public editing API or an autonomous bot account.

## Enable and authorize

1. Deploy with an exact HTTPS `TECHPEDIA_PUBLIC_URL`.
2. Set `TECHPEDIA_MCP_ENABLED=true` and rebuild/restart.
3. Sign in as a current admin or owner and open `/admin/connections`.
4. Register the client using the integration's actual redirect URL.
5. Authorize `wiki:read` and, only if desired, separately consent to `wiki:propose`.

Read-only grants are never silently upgraded. Revocation belongs to the same
administrator screen. Current permission and immutable account identity are
rechecked for requests and refresh; a removed admin does not retain access.

## Read and propose

Read scope provides public TechMark source and history. Proposal scope can submit
document creation, edits, and language links. Partial text replacements and
replacement of a pending proposal are supported; accepted edits still become
ordinary whole-document revisions after review.

Review proposals at `/admin/proposals`: inspect the exact diff or language link,
then explicitly approve or reject with a current administrator cookie session.
Batch review/approval is still human authorization. Snapshot conflicts require a
fresh proposal rather than silently overwriting newer content.

The MCP endpoint cannot delete documents, unlink, merge, approve, or directly
commit changes. A model's confirmation text or tool annotation is not permission
to publish. Deletion remains owner-only outside MCP.

## Limits and troubleshooting

Proposals accept up to 2 MiB of UTF-8 source; HTTP and rendering budgets also
apply. Retention defaults to two days and administrators can choose one, two or
three days. The former small queue-count cap has been removed. An old queue-full
error can indicate an outdated deployment; check the running version rather than
repeatedly submitting duplicates. If tools change after an update, refresh the client's tool
catalog using the client's supported mechanism; reconnect only if required.

Keep tokens and OAuth state out of issue reports and public exports.
