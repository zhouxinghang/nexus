# 06: Files — roots, scan, browse, metadata

**What to build:** The Owner manages the files on the Host through Nexus. This
ticket introduces the `Host` port's filesystem-read capability (fake backed by a
real temp dir in tests). The Owner configures one or more Managed Roots
(host directories), triggers scans that index files as Resources, and browses
them by root and subdirectory. Disk is the source of truth: a scan creates
Resources for new files, updates metadata for changed files, and marks vanished
files as Missing (kept, greyed out, manually clearable) so attached tags/notes
survive. The Owner can also register a single file by path. For each Resource
they see its metadata, add tags (and filter by tag), write a free-text note, and
download/open the underlying file. Metadata lives in Nexus, never written into
the file.

**Blocked by:** 01 (walking skeleton + auth)

**Status:** ready-for-agent

- [ ] The Owner can configure one or more Managed Roots; a path that doesn't exist or isn't a directory is rejected.
- [ ] The Owner can remove a Managed Root.
- [ ] A scan indexes files under a Managed Root as Resources; the Owner can trigger a rescan on demand.
- [ ] On scan: a newly appeared file becomes a new Resource; a changed file (size/mtime) has its Resource metadata updated; a file deleted outside Nexus is marked Missing (not auto-removed), keeping its tags/notes.
- [ ] Missing Resources are shown greyed out and can be manually cleared.
- [ ] The Owner can manually register a single file by its path.
- [ ] The Owner can browse Resources by Managed Root and subdirectory and see each Resource's name, path, size, type, and modified time.
- [ ] The Owner can add tags to a Resource and filter Resources by tag.
- [ ] The Owner can write a free-text note on a Resource.
- [ ] The Owner can download or open a Resource's underlying file.
- [ ] Tags and notes are stored by Nexus (not written into the file); the original file is never altered by metadata.
- [ ] There are no external-URL resources — every Resource points to a real file on the Host.
- [ ] Tests (through the HTTP API seam, with a fake `Host` filesystem backed by a real temp dir) assert: root config incl. rejection of bad paths, scan indexing, new/updated/Missing transitions, manual clear, single-file register, browsing/metadata, tag add+filter, note write.
