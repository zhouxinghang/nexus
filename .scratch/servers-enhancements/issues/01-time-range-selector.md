# 01: Time range selector

Status: ready-for-agent

**What to build:** Let the Owner choose the trend-chart window (1h / 24h / 7d /
30d) and keep long ranges readable. The backend already accepts `?from=&to=` on
`GET /api/servers/samples` (defaulting to 24h), but `web/src/pages/Servers.tsx`
sends no params and plots everything. Add the UI control and display-time
downsampling.

**Blocked by:** None

**Spec:** `.scratch/servers-enhancements/spec.md`

## Acceptance criteria

- [ ] Frontend adds a range selector with presets **1h / 24h / 7d / 30d** that
      computes `from`/`to` and passes them to `GET /api/servers/samples`; the
      charts reflect the chosen window.
- [ ] The selected preset persists in `localStorage` (mirror the sidebar-collapse
      pattern) and is restored on next visit.
- [ ] `GET /api/servers/samples` gains an optional downsampling param
      (`max_points` or `bucket`): when present, rows are averaged into time
      buckets in SQL (e.g. `date_bin` / `width_bucket`) so at most ~N points are
      returned for the range; when absent, raw rows are returned (back-compatible).
- [ ] The live current-values refresh (~5s) is unchanged; only the historical
      charts gain the range.
- [ ] Tests (`internal/httpapi`): seed samples across a span; assert `?from=&to=`
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
