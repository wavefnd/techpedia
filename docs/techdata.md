# Techdata and language links

Techdata provides language-independent identity separately from article content.

- A WID identifies one immutable language-specific document.
- A TID identifies a central item.
- A PID identifies a typed-statement property.
- An item can link at most one document per edition; a document belongs to at most
  one item.

These identifiers belong to authenticated administrator tools, not the public
reading interface. Translation relationships are not article fields or slugs.

## Link translations

Open the Techdata section with the appropriate permission. Create or choose the
central item, then attach each actual locale/document pair. Check both pages
before saving. Missing translations may be created and linked later; do not link
unrelated subjects merely because names look similar.

Labels, descriptions, and aliases describe an item per language. Typed statements
have defined value types and may include qualifiers and references. Item links
and their reverse indexes are updated in one database transaction; conflicting
locale membership fails rather than silently replacing another document.

## Cleanup

An owner may permanently delete an unused item by typing its exact TID in the
administrator confirmation. Deletion is blocked by attached documents, statements,
incoming references (including qualifier/reference snaks and quantity units), or
remaining reverse indexes. Deleted identifiers are never reused.

MCP can propose language links for administrator review; it cannot unlink, merge,
or commit a link directly. See [ChatGPT and MCP](integrations.md).
