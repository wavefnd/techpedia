# TechPedia embedded fonts

TechPedia serves its webfonts from the same origin as the application. The production browser never contacts a font CDN.

## Build-managed packages

Vite imports the following exact npm packages from `web/src/styles/fonts.css` and emits only their referenced WOFF2 files into the generated asset directory:

- Fontsource variable packages `5.3.0`: Inter, JetBrains Mono, Noto Sans JP, Noto Sans SC, Noto Sans TC, Noto Sans Arabic, Noto Sans Devanagari, Noto Sans Bengali, Noto Sans Hebrew, and Noto Sans Ethiopic
- Pretendard `1.3.9`: the official variable dynamic subset distribution

These styles use `unicode-range` and `font-display: swap`. A browser downloads only the slices needed by text present on the page, rather than every bundled language font.

The four Greek-only WOFF2 files under `web/src/assets/fonts/noto-greek` were copied from Fontsource Noto Sans variable `5.3.0`. They use a separate family name and Greek-only Unicode ranges so Noto renders Greek while Inter continues to render adjacent Latin and Cyrillic text. Their SHA-256 hashes, in italic-ext, normal-ext, italic, and normal order, are `69a4235a589cdd343dbb6e90264f22416cee4a8562975f2dbd5414f5c2b4a644`, `790b255c10c32e0b10bdcd961abad098c9697b7ffbabc1b09aca0b17af3201fd`, `5fc57904a663de085c84ba9d2adef65a9bc4abe0d756c1b445679a3302782029`, and `7f655207c2f6c140c327d6bd263bccc7a11833084f4fb0c51d9a24b7cd753651`.

## Generated CJK code subsets

`noto-cjk/subsets/` contains Unicode-range WOFF2 slices generated from the official Noto Sans Mono CJK KR variable TTF. They are used only as a fallback for characters inside code and the editor that JetBrains Mono does not contain. The browser selects only the 512-codepoint slices needed by the current code instead of downloading the full CJK font.

- Upstream repository revision: `f8d157532fbfaeda587e826d4cd5b21a49186f7c`
- Upstream path: `Sans/Variable/TTF/Mono/NotoSansMonoCJKkr-VF.ttf`
- Upstream TTF SHA-256: `0a2036b0470ad56d35776419e28361e81ce2766370b897e338678d2ce14be738`
- Variable weight range: 400–700
- Generator: `scripts/build-cjk-code-font.py` with FontTools 4.59.1 and Brotli 1.2.0
- Per-slice hashes: `noto-cjk/subsets/SHA256SUMS`

All font license texts are kept in `licenses/`. Noto CJK JP, SC, TC, and Mono share the Noto CJK SIL Open Font License text.
