# Embedded fonts

TechPedia serves bundled fonts from its own origin, without a production font CDN.
Interface/prose use `--font-sans`; inline code, code blocks and CodeMirror use
`--font-code` in `web/src/styles/tokens.css`.

| Coverage | Bundled family |
| --- | --- |
| Latin / Cyrillic | Inter |
| Korean | Pretendard |
| Japanese / Simplified / Traditional Chinese | Noto Sans JP / SC / TC |
| Arabic, Devanagari, Bengali, Hebrew, Ethiopic | Corresponding Noto Sans family |
| Greek | Greek-only Noto Sans slices |
| Code | JetBrains Mono |
| CJK code fallback | Noto Sans Mono CJK KR subsets |

System sans-serif/monospace remain final fallbacks; no fixed bundled selection
guarantees every Unicode glyph on every platform.

Fontsource packages and official Pretendard subsets are pinned in the web
lockfile. CJK code subsets come from a pinned official Noto source. Preserve
provenance, hashes, modification details and OFL licenses in
`web/public/fonts/README.md` and `web/public/fonts/licenses`.

Unicode ranges request slices needed by displayed characters. Faces use
`font-display: swap` so downloads do not block initial prose. The language profile
selects glyph priority, not a content filter. Verify mixed scripts, RTL, code,
line wrapping, mobile, self-hosted requests, Firefox and Chromium after changes.
Font license texts must accompany binary/container distributions, not just source.
