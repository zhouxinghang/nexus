# 01 — Resource content API and safe Host writes

Status: done

Implement the authenticated preview/content endpoints, bounded UTF-8 detection,
browser media streaming with Range support, Managed Root confinement,
revision-checked atomic saves, metadata refresh and edit auditing.

## Verification

- Host filesystem tests cover real temporary files, permission preservation,
  same-size/same-mtime conflicts, symlink escapes, internal read-only links,
  missing files, directories and oversized content.
- HTTP seam tests cover preview types, HTML/SVG source isolation, media bytes,
  Range responses, UTF-8 BOM/CRLF, metadata/tags/notes/audit, validation,
  authentication, missing files, download confinement and simultaneous saves.

## Comments

- 2026-10-14: implemented in the Host content seam and Files content handlers.
  `go test ./... -count=1` passed against ephemeral Postgres databases;
  focused content/download tests also passed with `-race`. `go vet ./...` passed.
