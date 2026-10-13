# Nexus

Nexus is a single-user personal workbench. It runs as one native Go process on a
host machine, serving a React UI and a JSON API, and gives its owner four tools
over that same host: a resource library, host monitoring, task tracking, and a
journal.

## Language

### Core

**Host**:
The single machine that runs the Nexus process and is also the machine being
monitored and whose files are managed. Nexus is single-machine by design.
_Avoid_: server, node, agent target

**Owner**:
The one person who uses this Nexus instance. There is exactly one.
_Avoid_: user, account, admin

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

**Missing** (resource state):
A Resource whose underlying file has disappeared from disk (deleted by something
other than Nexus). Kept, not auto-removed, so its tags and notes survive; shown
greyed out until the Owner clears it. Disk is the source of truth for existence.
_Avoid_: orphaned, stale, dangling

### Servers (host monitoring)

**Metric Sample**:
One timestamped reading of the Host's vitals (CPU, memory, disk, network, load,
uptime, process list, Docker container states), persisted to build history.
_Avoid_: datapoint, stat, reading

### Tasks

**Task**:
A unit of work with a fixed status of Todo, Doing, or Done, plus an optional
due date. Tasks are ordered within each column by drag-and-drop.
_Avoid_: todo, ticket, issue, card

### Journal

**Journal Entry**:
An append-only, immutable, plain-text record stamped with its creation time.
Cannot be edited or deleted. Searchable by case-insensitive substring match
(works for Chinese and other non-space-delimited text; see ADR-0004).
_Avoid_: note, log, post, entry
