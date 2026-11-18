# TechMark reference

TechMark is TechPedia's source format, not MediaWiki wikitext. The server renders
safe HTML and the browser progressively enhances it. Source is retained in revisions.
A page's creation-time name, not a `#` heading, determines its title and URL.

## Text and headings

```text
# Introduction

**Bold**, *italic*, ~~struck through~~, and `inline code`.

## Initially expanded section
Normal paragraphs and [[Operating System|internal links]].

!## Initially collapsed section
### Subheading
#### Detail
```

Use a space after heading markers. Level-two sections can be collapsed; their
children remain in the same section until the next level-two heading. Anchors and
the table of contents open collapsed sections when navigating to them. Headings
clear floating infoboxes and side images by default.

A standalone line containing three or more hyphens is a horizontal rule. In
navboxes, tables, and code, hyphens keep their local meaning. Fenced code and inline
code preserve syntax examples as text, not executable wiki components.

## Links and references

`[[Document name]]` links within the current edition; `[[Document name|label]]`
sets display text. Spaces become underscores in URLs. Reserved characters are
percent-encoded: `[[C#]]` links to `C%23`, and `[[A/B]]` to `A%2FB`.
Only the initial Latin character of a target is capitalized; other case is retained.
Missing links lead toward creation with the normalized name. Language switches
use Techdata rather than guessed translated names.

External links use `[label](https://example.com)`. Only HTTP(S) links are accepted.
For references, place a footnote immediately after the claim:

```text
A documented claim.<wiki-footnote>[Source](https://example.com)</wiki-footnote>
```

Footnotes are numbered in encounter order and collected below the article,
outside collapsible sections. Empty notes are invalid. Notes cannot cross fenced
code blocks. Old `[^name]` syntax has read compatibility; use `wiki-footnote`
for new source.

## Code and tables

Use triple-backtick fences with an optional language such as `go`, `cpp`, or
`bash`. Code is escaped and can be highlighted with line numbers. It does not
execute tags or links.

```text
| Name | Role |
| --- | --- |
| [[Go]] | Implementation language |
| BadgerDB | Embedded storage |
```

## Alerts and infoboxes

```text
<wiki-alert type="warning">
This article needs a source.
</wiki-alert>

<wiki-infobox title="Go" type="language">
<wiki-image file="Logo.webp" alt="Go logo">Project logo</wiki-image>
| Developer = Google
| First release = 2009
| Related = [[Programming language]]
</wiki-infobox>
```

Alert types are `info`, `warning`, `danger`, `success`, and `note`.
Infobox rows use `| Label = Value`. Images can appear between rows and fit the box
regardless of their layout attribute. Keep long explanations in article text.

## Navigation boxes

```text
<wiki-navbox title="Operating systems" state="expanded" accent="#334455">
- UNIX:
-- BSD: [[FreeBSD]] · [[OpenBSD]]
-- Linux {color="#E95420" note="Kernel"}: [[Linux]]
------ Distributions: [[Debian]] · [[Ubuntu]]
</wiki-navbox>
```

A row starts with one or more hyphens followed by a space or tab. A row belongs
to the nearest earlier row with fewer hyphens. Missing intermediate depths do not
create synthetic rows. Arbitrary deeper levels retain the hierarchical table;
the engine does not add numbered categories. Mobile uses a stacked layout.

`state` accepts `expanded` (default) or `collapsed`. `accent` sets the title
color and a row's `color` sets its label cell. Only six-digit HEX colors are
accepted; foreground contrast is selected automatically. Labels are plain text;
contents support inline links and formatting. Arbitrary style/event attributes
are not allowed.

## Trees and flows

```text
<wiki-tree title="Family" state="expanded">
UNIX
├── BSD
│   └── [[FreeBSD]]
└── Independent implementations
    └── [[Linux]]
</wiki-tree>

<wiki-flow title="Lifecycle">
Acquire
↓
Use
↓
Release
</wiki-flow>
```

Tree branch tokens and groups of four indentation spaces encode structure rather
than literal code art. Multiple roots are supported. Nodes are not automatically
links. A flow uses a standalone `↓` between steps; `⇣` and `▼` are accepted
aliases. Both components support optional title, state, and six-digit accent.

## Images

```text
<wiki-image file="Desktop.webp,Alternate.webp" layout="side" alt="Desktop examples">
Two views of the desktop
</wiki-image>
```

Use exactly one of `file` (uploaded filename) or `src` (one external HTTPS URL).
Layouts are `frame`, `side`, and `wide`. Translate descriptive alt text and
captions, not filenames. Captions support inline TechMark. External images show
an origin credit and are not downloaded into server storage.

Multiple uploaded files form a horizontal, non-autoplay carousel with arrows,
position indicator, keyboard controls and mobile scrolling. Order and duplicates
are retained. The side layout is 44% wide up to 440px on desktop and full width
on mobile. Clicking opens an enlarged view. Printing shows every image.

Spaces around entries are ignored; empty or invalid entries reject the whole
block. Escape a comma inside a filename as `\,`. A comma in external `src`
is not a separator. The same syntax works inside infoboxes.

Resource safety limits are 64 files per image block, 1,024 images per document,
and 8 MiB of generated image markup per document. Infobox images share the same
document budget. Exceeding a document budget displays an explicit rendering error,
not a silently truncated article. These bounds limit generated output, not prose
length. Use separate articles or fewer redundant images for unusually large sets.

## Reusable boxes

```text
<wiki-box name="Operating systems" />
```

A bodyless paired tag is also supported. Only `name` is accepted; there is no
argument substitution or article transclusion. Escape `&` in attributes as
`&amp;`. See [boxes and media](boxes-and-media.md) for language fallback,
source restrictions, expansion limits, and current-box behavior in old revisions.

## Administrator-only system components

These tags are reserved operational syntax, not ordinary editor components:
`wiki-recent-changes`, `wiki-random-page`, `wiki-home-grid`,
`wiki-featured-pages`, and `wiki-popular-pages`. They require an administrator
and an `admins` edit policy. Homepage panels additionally require the configured
main page. Tags inside code are examples, not operations.

```text
<wiki-home-grid>
<wiki-featured-pages title="Featured" limit="3" empty="No articles"></wiki-featured-pages>
<wiki-popular-pages title="Popular" period="7d" limit="10" empty="No views"></wiki-popular-pages>
</wiki-home-grid>
```

Featured panels default to 3 entries, popular panels to 10; both cap at 12.
Popular periods are `1d`, `7d`, and `30d`. Featured selection is deterministic
per UTC day and edition. Main pages and operational pages are excluded. These
panels do not fetch news. Use localized titles and empty messages.

## Safety

Only supported safe markup is rendered. JavaScript URLs, arbitrary scripts,
event handlers, and unrestricted CSS are not accepted. Do not rely on raw HTML
being a stable extension API. Preserve source, preview changes, and use tests
when extending syntax.
