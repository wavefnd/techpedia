# Security policy

## Private reporting

Do not post vulnerabilities, passwords, tokens, or private document/user data in
public issues or pull requests. Contact **luna@lunastev.org**, the engine's existing
security contact. An individual wiki operator may have a different contact.

Include the affected version/commit, required permissions and configuration,
minimal reproduction steps, and expected impact. Describe credential types and
locations without sending their values. Test only an isolated installation you
control; do not stress-test or probe another operator's service.

## Boundaries

- Anonymous readers, editors, Techdata operators, admins, and the single owner
  have different permissions. Document deletion is owner-only.
- MCP OAuth read/proposal scopes never grant administrator approval authority.
- Document, proposal, box, and external translation content is untrusted input.
- The server-side TechMark renderer is a primary HTML safety boundary. CSP in
  report-only mode and Trusted Types do not replace safe rendering.
- Runtime databases, uploaded images, backups, `.env`, and OAuth state must not
  be included in public source packages.

Maintainers review confirmed fixes and decide releases. No fixed response time
or long-term support for every historical version is promised. Operators must
rebuild and deploy fixed binaries/images: updating Git alone does not replace a
running service. Public contribution CI must never receive production secrets.
