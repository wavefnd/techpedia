# Third-party notices

Copyright 2026 Wave Foundation.

The [Blue Oak Model License 1.0.0](LICENSE) applies to original TechPedia engine source, not to
third-party software, fonts, language metadata, or an installation's wiki content.
Those components retain their original copyright and license conditions.

## Dependency inventory

Resolved versions are recorded in `go.mod`, `go.sum`, `web/package-lock.json`, and
`docs/package-lock.json`. The documentation toolchain is separate from the wiki
runtime. Run `go run ./scripts/notices` after installing web dependencies and
downloading Go modules to collect dependency license/NOTICE texts into
`build/licenses`. The output includes an inventory and flags missing texts for
human review; a version list alone is not a license clearance.

Include generated notices, this file, and LICENSE with redistributed binaries
and containers. Preserve notices for local modifications to upstream material.
The container build collects runtime dependency notices automatically.

## Fonts and metadata

Font provenance, versions, hashes, and the CJK subset modification procedure are
documented in [web/public/fonts/README.md](web/public/fonts/README.md). Original
SIL Open Font License texts and copyrights remain in `web/public/fonts/licenses`.
Do not remove these notices from distributions.

The IANA language subtag registry under `internal/edition/data` records its own
source and date. CLDR-derived language/direction metadata is independent of an
operator's edition acceptance policy. Preserve upstream attribution.

The engine license does not license a wiki's articles, uploaded media, user data,
or third-party trademarks. No production content is included in a public export.
