# Configuration

The native binary reads exported environment variables. The deployment scripts
parse `.env` as literal key/value pairs without executing shell code. Exported
variables take precedence. Do not use shell substitutions inside `.env`.

## Engine variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `TECHPEDIA_ADDR` | `0.0.0.0:8080` | Native HTTP listener; Compose sets the internal listener |
| `TECHPEDIA_DATA_DIR` | `data` | Native runtime root; Compose mounts `./data` at `/data` |
| `TECHPEDIA_PUBLIC_URL` | `http://localhost:8080` | Exact public origin for canonical URLs and OAuth/origin checks |
| `TECHPEDIA_CSP_MODE` | `report-only` | Set `enforce` only after testing actual policy compatibility |
| `TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME` | empty | Single immutable owner identity |
| `TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD` | empty | Initial password only; does not reset an existing account |
| `TECHPEDIA_MEDIA_STORAGE_LIMIT_BYTES` | `943718400` | Global media storage budget |
| `TECHPEDIA_MEDIA_UPLOAD_LIMIT_BYTES` | `10485760` | Per-upload byte limit |
| `TECHPEDIA_INTERFACE_AUTO_TRANSLATION` | `true` | External interface draft translation |
| `TECHPEDIA_INTERFACE_TRANSLATION_URL` | MyMemory endpoint | Translation service endpoint |
| `TECHPEDIA_INTERFACE_TRANSLATION_EMAIL` | empty | Optional provider contact address |
| `TECHPEDIA_ANALYTICS_ENABLED` | `true` | Built-in aggregate traffic statistics |
| `TECHPEDIA_ANALYTICS_COUNTRY_HEADER` | `CF-IPCountry` | Trusted-proxy country hint |
| `TECHPEDIA_MCP_ENABLED` | `false` | Administrator-only OAuth/MCP integration |
| `TECHPEDIA_SERVER_LOCATION` | empty | Optional administrator-visible server location |

The example file disables external translation for an explicit opt-in. Engine
fallback defaults apply when a variable is omitted. Keep the bootstrap username
unchanged after setup; use account management rather than changing the environment
to reset a password.

## Docker and HTTPS

`TECHPEDIA_ENABLE_HTTPS=true` enables the production Caddy profile in `start.sh`.
Set `TECHPEDIA_DOMAIN`, `TECHPEDIA_TLS_EMAIL`, and
`TECHPEDIA_PUBLIC_URL=https://your-domain` together. DNS must point at the server.
`TECHPEDIA_HTTP_BIND=127.0.0.1` keeps the backend host port private.

An external reverse proxy may terminate HTTPS instead. Keep the public origin
HTTPS, disable the bundled Caddy profile, and restrict backend access to the proxy.
Do not accept forwarded country/security headers directly from arbitrary clients.
See [deployment](deployment.md) and [security](security.md).
