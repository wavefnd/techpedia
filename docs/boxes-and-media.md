# Reusable boxes and media

## Boxes

Manage named resources at `/admin/boxes`. A box is not a wiki article or a
programmable template. Give it a stable name and one TechMark body per language.
Insert it on its own line:

```text
<wiki-box name="Operating systems" />
```

The current edition's body is used. A missing language stays hidden unless an
administrator configures a fallback. The fallback retains its real link language.
Copying another language creates an unsaved draft: only actual Techdata-linked
targets can be remapped; labels and prose still need translation.

Box source cannot contain nested boxes, headings, footnotes, or operational
reserved tags. Source is limited to 128 KiB per language; a document expands at
most 64 calls and 2 MiB of box HTML. Box updates affect subsequent reads of all
references, including historical article revisions. Article history stores the
call, not a frozen copy of the box.

## Upload and insert

Users with `edit` can upload through the image insertion dialog. Files use a global
media library rather than per-edition storage. The server validates and converts
accepted images to WebP variants. Reuse the same file across languages and translate
only its alt text and caption.

```text
<wiki-image file="Desktop.webp" layout="side" alt="Desktop with an application menu">
The default desktop
</wiki-image>
```

Multiple files use a comma-separated list and a horizontal carousel, not a vertical
stack. See [image syntax and limits](techmark.md#images).

The library offers upload-file deletion with permission and reference checks.
Deleting a file is different from removing a tag in an article; current box
references also prevent permanent removal. Review the confirmation and references
before deleting. Keep an offline backup of irreplaceable originals.

External images use an HTTPS `src` and are not copied into the media library.
They can reveal a reader's request to the remote host. Prefer an appropriately
licensed local upload where privacy and availability matter.
