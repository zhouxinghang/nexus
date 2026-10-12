# 07: Files — mutate underlying files

**What to build:** The Owner reorganizes the Host's filesystem from the UI. They
rename a Resource's underlying file, move it to another location, or delete it.
Operations are confined to Managed Roots: a rename/move whose target falls
outside every Managed Root is rejected. Deletes are permanent — the file is
removed, no trash. Tags and notes follow a file through a rename or move, and
deleting a file's bytes removes/marks its Resource so no dangling entry remains.
Every mutation is recorded in an audit log the Owner can review.

**Blocked by:** 06 (files — roots, scan, browse, metadata)

**Status:** ready-for-agent

- [ ] The Owner can rename a Resource's underlying file on disk.
- [ ] The Owner can move a Resource's underlying file to another location.
- [ ] The Owner can delete a Resource's underlying file.
- [ ] A rename/move whose target falls outside every Managed Root is rejected with a clear client error.
- [ ] Delete permanently removes the file (no trash directory).
- [ ] Tags and notes follow a file through rename and move.
- [ ] Deleting a file's bytes removes or marks its Resource so no dangling entry remains.
- [ ] Every file mutation (rename/move/delete) is recorded in an audit log.
- [ ] Tests (through the HTTP API seam, with a fake `Host` filesystem backed by a real temp dir) assert: rename, move, out-of-root rejection, permanent delete, tags/notes surviving rename/move, Resource cleanup on byte deletion, and audit-log entries for each mutation.
