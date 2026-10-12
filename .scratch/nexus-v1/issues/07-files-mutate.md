# 07: Files — mutate underlying files

**What to build:** The Owner reorganizes the Host's filesystem from the UI. They
rename a Resource's underlying file, move it to another location, or delete it.
Operations are confined to Managed Roots: a rename/move whose target falls
outside every Managed Root is rejected. Deletes are permanent — the file is
removed, no trash. Tags and notes follow a file through a rename or move, and
deleting a file's bytes removes/marks its Resource so no dangling entry remains.
Every mutation is recorded in an audit log the Owner can review.

**Blocked by:** 06 (files — roots, scan, browse, metadata)

**Status:** done

- [x] The Owner can rename a Resource's underlying file on disk.
- [x] The Owner can move a Resource's underlying file to another location.
- [x] The Owner can delete a Resource's underlying file.
- [x] A rename/move whose target falls outside every Managed Root is rejected with a clear client error.
- [x] Delete permanently removes the file (no trash directory).
- [x] Tags and notes follow a file through rename and move.
- [x] Deleting a file's bytes removes or marks its Resource so no dangling entry remains.
- [x] Every file mutation (rename/move/delete) is recorded in an audit log.
- [x] Tests (through the HTTP API seam, with a fake `Host` filesystem backed by a real temp dir) assert: rename, move, out-of-root rejection, permanent delete, tags/notes surviving rename/move, Resource cleanup on byte deletion, and audit-log entries for each mutation.

## Comments

- API: `POST /api/files/resources/{id}/rename` (bare filename within the same dir), `POST …/move` (absolute dest; a dir dest keeps the name), `DELETE …/file` (permanent `os.Remove`, no trash), and `GET /api/files/audit`.
- Rename/move share `doMove`, which validates the target is inside a Managed Root (rejected with 400 otherwise), performs the real syscall via `Host.Rename`, updates the Resource row in place (so tags/notes — keyed by resource id — follow automatically), and writes an audit entry. Delete removes the Resource row and audits the action.
- Tests in `internal/httpapi/files_mutate_test.go` cover rename, move into a subdir, out-of-root rejection (and that the original file is undisturbed), permanent delete + no dangling Resource, tags/notes surviving rename and move, and that the audit log records rename/move/delete.
