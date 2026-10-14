# Files — preview and text editing

Confirmed request: 2026-10-14 (Asia/Shanghai)

Continue the FileBrowser Quantum reference and add previewing Resources and
editing the underlying text files, without adding another tool or service.

## Reference and scope

Read the official FileBrowser Quantum
[Previewing Files](https://filebrowserquantum.com/en/docs/features/previewing-files/)
documentation on 2026-10-14. Borrow its type-aware viewer, source editor with
line numbers/search/save, full-window image viewing and adjacent-file navigation.
This is a scoped Nexus implementation, not feature parity: no OnlyOffice,
FFmpeg, office conversion, EPUB/3D viewer, collaboration or external service.
Quantum's documented text limit is 25 MB; Nexus deliberately uses a bounded
2 MiB UTF-8 text limit. Markdown, HTML and SVG are shown as source, not rendered
as executable same-origin documents.

## Acceptance

- Double-click a present non-directory Resource, press Enter on its row, or
  choose “Preview / edit” in its detail panel.
- Images have fit/zoom controls; video/audio use native browser controls;
  PDFs use the browser's embedded PDF viewer. Support depends on browser codecs.
- Unsupported formats, non-UTF-8/binary data and oversized text have an explained
  download fallback. Missing Resources and directories cannot be edited.
- Text preview is read-only by default. Explicitly switch to Edit to use the
  locally bundled CodeMirror editor, syntax highlighting, line numbers,
  search/replace, undo/redo, optional wrapping and Ctrl/Cmd+S.
- UTF-8 BOM and consistent LF/CRLF/CR line endings round-trip. Mixed line
  endings are explicitly labeled “Mixed → LF” and normalized only upon editing.
- Closing, reloading, changing files or returning to Preview confirms unsaved
  changes; page refresh/unload also warns. Saving locks editing/navigation.
- A SHA-256 revision of disk bytes prevents stale editors overwriting later
  edits, including same-size edits with unchanged timestamps. Conflicts retain
  the draft, offer a local draft download and require reload, not force-save.
- New operations and the download fallback are confined to the Resource's
  actual Managed Root through `os.Root`. Escaping symlinks are rejected;
  readable internal symlinks are read-only. Only regular files are opened.
- Save stages/syncs a same-directory temporary file and renames it atomically,
  preserves permission bits, refreshes Resource size/mtime and records an
  `edit` audit entry. Resource identity, tags and notes are unchanged.
- Native dialog plus explicit edge focus cycling, Escape, focus restoration,
  responsive layout, semantic tokens and reduced-motion support.

## API

- `GET /api/files/resources/{id}/preview`: current disk metadata, preview type,
  MIME, editability and fallback reason.
- `GET /api/files/resources/{id}/content`: text JSON
  `{content, revision, encoding}`, or inline media with Range support.
- `HEAD /api/files/resources/{id}/content`: media metadata.
- `PUT /api/files/resources/{id}/content`: `{content, revision}`; returns updated
  text JSON. Stale revision → 409, decoded text over 2 MiB → 413,
  binary/read-only/directory → 415, unsafe path → 403, vanished file → 404.
- Every endpoint is under the existing Owner session authentication.

## Storage boundary

Disk replacement and Postgres commit cannot be one transaction. If the bytes
were saved but metadata/audit commit fails, return an explicit error telling the
Owner to reload or scan; never imply that the disk write was rolled back.
The hash is checked again immediately before rename, but another Host process
can still write in the final check/rename window; this is not an OS-wide lock.
Replacement preserves mode bits, not hard-link identity or all extended metadata.
