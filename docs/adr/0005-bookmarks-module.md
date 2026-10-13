# Bookmarks are the one place external URLs live; Files stays local-only

The Files module is deliberately scoped to real files on the Host's filesystem:
every Resource points at a path inside a Managed Root, and the "no external-URL
resources" rule (see the v1 spec and CONTEXT.md) keeps file management honest —
tags, notes, scans, and mutations all assume a byte stream on disk.

Website navigation is a genuinely different need: a named, clickable link to an
external site, with periodic reachability checks. Rather than overload Resource
with a second "kind" (which would reintroduce the dropped "Link" concept and
blur the Files invariant), Bookmarks is its own module and `Bookmark` is its own
entity. A Bookmark has a name, an http(s) URL, an optional icon, and a health
state produced by a background checker.

Consequences:

- Files keeps its local-only invariant intact; nothing in Files learns about
  URLs. The two modules share nothing but the chrome.
- Bookmark health is checked on a fixed interval (every 5 minutes) through a
  `Prober` seam, mirroring the Servers sampler: a real HTTP prober in
  production, an injected fake in tests so no live network calls happen. A probe
  that returns any status below 500 counts as `up`; a transport failure or 5xx
  is `down`.
- Active control of the sites (beyond opening them in a new tab and probing
  reachability) is out of scope, consistent with ADR-0002's single-host,
  monitoring-not-controlling posture.
