# Nexus

Nexus is a single-user personal workbench. It runs as one native Go process on a
host machine, serving a React UI and a JSON API, and gives its owner five tools
over that same host: a resource library, host monitoring, task tracking, a
journal, and a website-navigation board.

## Language

### Core

**Host**:
The single machine that runs the Nexus process and is also the machine being
monitored and whose files are managed. Nexus is single-machine by design.
_Avoid_: server, node, agent target

**Owner**:
The one person who uses this Nexus instance. There is exactly one.
_Avoid_: user, account, admin

**Overview**:
The landing page: a read-only summary of the whole workbench (Host vitals with
visual threshold flags, Task counts, latest Journal Entries, down Bookmarks,
Files totals), each panel linking into its tool. It stores nothing of its own
and never notifies.
_Avoid_: home, dashboard, index

### Files

**Resource**:
One indexed entry in the Files module, pointing to a real file on the Host's
filesystem. Nexus stores its metadata plus Owner-added tags and notes, and can
rename, move, or delete the underlying file (operations confined to inside a
Managed Root). There are no external-URL resources.
_Avoid_: document, material, asset, attachment, file, link

**Managed Root**:
A Host directory the Owner has configured for Nexus to scan and index. All file
operations are confined to inside Managed Roots; a move or rename targeting a
path outside every Managed Root is rejected.
_Avoid_: folder, workspace, mount

**Scan**:
Walking a Managed Root's directory tree and reconciling it with the index:
new/changed files are upserted (a file is "changed" only if its size or
modified time differs — unchanged files are skipped), and present Resources no
longer on disk become Missing. A Scan runs asynchronously in the background so
the Host never blocks on a large tree: starting one returns a running status
immediately, and progress (files seen, indexed, removed) is polled until it
reaches `completed` or `failed`. Only one Scan runs per Managed Root at a time.
_Avoid_: reindex, sync, crawl, refresh

**Missing** (resource state):
A Resource whose underlying file has disappeared from disk (deleted by something
other than Nexus). Kept, not auto-removed, so its tags and notes survive; shown
greyed out until the Owner clears it. Disk is the source of truth for existence.
_Avoid_: orphaned, stale, dangling

### Servers (host monitoring)

**Metric Sample**:
One timestamped reading of the Host's vitals (CPU, memory, disk, network
counters and throughput, load, pressure stall on Linux, uptime, Nexus's own
CPU/memory, process list, Docker container states), persisted to build history.
_Avoid_: datapoint, stat, reading

### Tasks

**Task**:
A unit of work with a fixed status of Todo, Doing, or Done, plus an optional
free-text description, a priority flag (None/Low/Medium/High/Urgent), an
optional due date, any number of Labels, and an ordered checklist of Subtasks.
Tasks are ordered within each column by drag-and-drop; priority and labels are
visual flags/filters, not ordering keys. There is no project or list grouping —
Labels are the one grouping mechanism.
_Avoid_: todo, ticket, issue, card

**Archived** (task state):
A Task the Owner has cleared off the board without deleting it. Hidden from the
board, listed newest-archived first in the archived view, and restorable to the
bottom of its column. Distinct from delete, which is permanent.
_Avoid_: closed, hidden, trashed

**Label**:
A named, colored tag the Owner attaches to any number of Tasks (many-to-many)
to categorize work across columns and filter the board. Renaming or recoloring
a Label updates every Task carrying it; deleting a Label detaches it from its
Tasks but never deletes them.
_Avoid_: tag (that's the Files term), category, project, list

**Subtask**:
One item in a Task's ordered, single-level checklist: a title and a done flag.
The Task's card shows progress as done/total. Completing every Subtask does not
change the Task's status.
_Avoid_: checklist item, child task, step

### Journal

**Journal Entry**:
An append-only, immutable, plain-text record stamped with its creation time.
Cannot be edited or deleted. Searchable by case-insensitive substring match
(works for Chinese and other non-space-delimited text; see ADR-0004).
_Avoid_: note, log, post, entry

### Bookmarks

**Bookmark**:
One entry on the website-navigation board: a name, a clickable external
http(s) link, and an optional icon (emoji or image URL). The Owner's name and
icon are authoritative; Nexus also keeps the site's own fetched title and
favicon separately, using the title as the name when the Owner leaves it blank
and the favicon as the default icon. Each Bookmark keeps a running count of
how often the Owner opened it. Unlike a Resource, a Bookmark points at
an external site, not a local file — this is the one place external URLs live,
and it keeps the Files module local-only (see ADR-0002, ADR-0005).
_Avoid_: link, favorite, shortcut, resource

**Health** (bookmark state):
The reachability of a Bookmark's site, recorded by a periodic health check:
`up` (last probe got an HTTP status below 500), `down` (the probe failed or
returned 5xx), or `unknown` (never checked yet). Each check also stores the last
status code, latency, and check time.
_Avoid_: status, uptime, ping
