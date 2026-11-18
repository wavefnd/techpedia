# Contributing

Read the repository's
[contribution guide](https://github.com/wavefnd/techpedia/blob/main/CONTRIBUTING.md)
and [repository rules](https://github.com/wavefnd/techpedia/blob/main/AGENTS.md).
Keep changes focused and tested. Report private security issues through
[SECURITY.md](https://github.com/wavefnd/techpedia/blob/main/SECURITY.md).

Documentation and operational script messages use English. The administrator UI
remains Korean, and public interface packs/content remain multilingual. This
documentation site is isolated from the Go-embedded reader frontend.

Before submitting, build web assets, run Go tests/vet, frontend typecheck/tests,
and the documentation build. Do not commit generated assets, dependencies,
credentials or runtime data. Version changes are maintainer release decisions.

For implementation boundaries, start with [architecture](arch.md) and
[TechMark](techmark.md). For redistribution, preserve the [license](license.md)
and third-party notices.
