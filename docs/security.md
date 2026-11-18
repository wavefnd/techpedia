# Secure operation

Use TLS and the exact public origin. Restrict the backend listener to your reverse
proxy, protect `.env`, and keep runtime data and backups outside public hosting.
The repository's [reporting policy](https://github.com/wavefnd/techpedia/blob/main/SECURITY.md)
describes private vulnerability reporting.

## Authentication and permissions

Passwords use Argon2id. Password work has a shared two-operation concurrency
budget and a token bucket (12 initial operations, replenishing at one per second).
Saturation returns HTTP 429 and Retry-After instead of queuing memory-hard work.
A signup can create the account but defer automatic login under saturation; the
response explicitly asks the user to sign in rather than registering again.

Login/signup require JSON. Foreign Origin, null Origin, and cross-site browser
requests are rejected before credentials or cookies are changed. Logout also
checks origin. Non-browser JSON clients without Origin remain supported.
Keep a reverse-proxy rate limit as another layer; process-local budgets are not
a network-wide abuse prevention service.

Sessions use HttpOnly, SameSite=Lax cookies; HTTPS deployments use secure cookies.
The Go API enforces permissions independently of Vue guards. One immutable owner
is selected by bootstrap identity and cannot grant ownership to another account.

## Rendering and concurrent writes

Untrusted TechMark is server-rendered. Closing-tag lookups are indexed instead of
rescanning every suffix, and galleries have per-block and document-wide budgets.
Do not remove those bounds to accommodate hostile or accidentally generated input.

Saving or restoring must name the revision actually observed during authorization.
A future revision token is not permission to overwrite a concurrent protected
revision. The database's atomic compare-and-swap remains the final write boundary.

## CSP and proxy policy

Start with report-only CSP, inspect real browser reports without logging sensitive
source, then test enforced mode before rollout. Do not treat report-only as an
active blocker. Keep Trusted Types and safe server rendering enabled.

Only trust country headers when your proxy replaces them and clients cannot bypass
it. Aggregate country/latency measurements are hints, not exact user geolocation
or ICMP ping. Avoid publishing raw logs, tokens, source, or personal information.

Backups contain private databases and may contain TLS material. Encrypt and
restrict them, test recovery offline, and apply retention deliberately.
