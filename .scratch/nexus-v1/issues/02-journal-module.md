# 02: Journal module

**What to build:** The Owner can keep an append-only journal. From the Journal
view they type plain text and append a Journal Entry, which is automatically
stamped with its creation time and shown at the top of a newest-first stream.
They can full-text search their entries by content. Entries cannot be edited or
deleted — the Journal is a trustworthy, immutable record (ADR-0003).

**Blocked by:** 01 (walking skeleton + auth)

**Status:** ready-for-agent

- [ ] The Owner can append a plain-text Journal Entry through the API and UI.
- [ ] Each entry is automatically stamped with its creation time; the Owner does not supply a timestamp.
- [ ] Entries are displayed newest-first as a stream.
- [ ] Full-text search over entry text (Postgres full-text search) returns matching entries.
- [ ] There is no edit and no delete path — the API offers neither, and the UI exposes neither.
- [ ] Entries carry no tags.
- [ ] Tests (through the HTTP API seam) assert: append works, timestamps are auto-set, newest-first ordering, search finds entries by content, and edit/delete are not possible.
