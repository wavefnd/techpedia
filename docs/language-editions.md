# Language editions

## Accept and activate an edition

Manage editions at `/admin/languages`. Editors may propose a registered base
IANA language code. An administrator accepts or rejects each proposal; acceptance
does not automatically activate general editing.

1. Review the proposed language code and the intended editorial policy.
2. Accept the proposal to configure it.
3. Review its interface-message and navigation draft.
4. Create and assign its administrator-controlled main page.
5. Activate the edition. Ordinary articles can then be created and edited.

The example codes `ko` and `en` are not a closed list. Codes are lowercase
base language subtags, such as `zh`; `zh-Hans`, region tags, variants, extensions,
and private-use tags are not edition identifiers. Administrators decide acceptance
and any related-language editorial policy; the engine does not merge languages.

## Interface translation

When automatic translation is enabled, accepting an edition can prefill editable
interface messages from the engine's fixed English seeds. The current default
provider is MyMemory, not a bundled translation model. It needs network access,
has provider limits, and offers no guarantee of translation quality or availability.
Set `TECHPEDIA_INTERFACE_AUTO_TRANSLATION=false` to avoid these external requests.

Review drafts before saving. Never translate navigation `Target` values as if
they were display labels: a target is a real document name or route. Use the
actual translated document and its Techdata relationship. Preserve manual edits.

## Writing direction and Unicode

IANA/CLDR metadata helps choose direction and font families. It is not a character
filter. Articles can contain foreign names, code, symbols, and arbitrary Unicode.
Use the same edition code in URLs, HTML language metadata, and Techdata keys.

Language switching follows actual Techdata-linked documents. It does not guess
a title from the current name, URL suffix, or translated navigation label.
