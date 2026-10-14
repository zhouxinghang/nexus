# 01: Time range selector

Status: done

**What to build:** Let the Owner choose the trend-chart window (1h / 24h / 7d /
30d) and keep long ranges readable. The backend already accepts `?from=&to=` on
`GET /api/servers/samples` (defaulting to 24h), but `web/src/pages/Servers.tsx`
sends no params and plots everything. Add the UI control and display-time
downsampling.

**Blocked by:** None

**Spec:** `.scratch/servers-enhancements/spec.md`

## Acceptance criteria

- [x] Frontend adds a range selector with presets **1h / 24h / 7d / 30d** that
      computes `from`/`to` and passes them to `GET /api/servers/samples`; the
      charts reflect the chosen window.
- [x] The selected preset persists in `localStorage` (mirror the sidebar-collapse
      pattern) and is restored on next visit.
- [x] `GET /api/servers/samples` gains an optional downsampling param
      (`max_points` or `bucket`): when present, rows are averaged into time
      buckets in SQL (e.g. `date_bin` / `width_bucket`) so at most ~N points are
      returned for the range; when absent, raw rows are returned (back-compatible).
- [x] The live current-values refresh (~5s) is unchanged; only the historical
      charts gain the range.
- [x] Tests (`internal/httpapi`): seed samples across a span; assert `?from=&to=`
      returns only in-range rows; assert the downsampling param caps the returned
      point count while preserving range coverage and ascending order; assert
      absence of the param returns raw rows.

## Notes

- This introduces **query-time aggregation for display**, which is explicitly
  distinct from the spec's "no downsampling/rollups" **storage** decision. The
  stored `metric_samples` and the 30-day retention are untouched — only the read
  path buckets points for rendering. Note this tension in the implementation and
  keep the raw path the default.
- Pick sensible per-range point caps (e.g. ~120–300 points) so each preset looks
  smooth without shipping tens of thousands of rows.
- Keep the chart X axis labels sensible per range (time-of-day for 1h/24h, date
  for 7d/30d).

## Comments

- Implemented: 1h / 24h / 7d / 30d control above the trend charts, persisted
  under `nexus-servers-range`. Each preset sends `from`/`to` plus a
  per-range `max_points` (120 / 288 / 336 / 360). X-axis labels show time of
  day for 1h/24h and dates for 7d/30d. The live 5s current-values refresh is
  unchanged.
- `GET /api/servers/samples?max_points=N` (≤2000, invalid → 400,
  `from` ≥ `to` → 400) buckets with `date_bin` aligned to `from`, width
  `span/N + 1µs`, so there are at most N points, ascending. Gauges are
  averaged; totals and cumulative counters (net, uptime) take the bucket max.
  Bucketed rows carry the bucket start as `taken_at` and empty
  `top_procs`/`containers`.
- **Tension, as flagged in the ticket:** this is query-time aggregation for
  display only. `metric_samples` and the 30-day raw retention are untouched,
  and without `max_points` the endpoint returns raw rows exactly as before.
  This is documented on the handler and the query.
