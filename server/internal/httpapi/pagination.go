package httpapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Paging conventions. Each list endpoint uses exactly one envelope:
//
//   - keyset: {"items": [...], "next_cursor": "<opaque>"|null} — for
//     append-only, stably ordered streams (the Journal).
//   - offset: {"items": [...], "total": n, "limit": n, "offset": n} — where
//     several optional filters make a stable keyset awkward (Files).
//
// Callers that pass no paging params get the first page. An invalid `limit`,
// `offset`, or `cursor` is a 400. A `limit` above the endpoint's cap is
// clamped to the cap rather than rejected.

type keysetPage[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type offsetPage[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

var errBadPaging = errors.New("invalid paging params")

// parseLimit reads ?limit=, defaulting to def and clamping to max.
func parseLimit(q url.Values, def, max int32) (int32, error) {
	v := q.Get("limit")
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n <= 0 {
		return 0, errBadPaging
	}
	if int32(n) > max {
		return max, nil
	}
	return int32(n), nil
}

// parseOffset reads ?offset=, defaulting to 0.
func parseOffset(q url.Values) (int32, error) {
	v := q.Get("offset")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n < 0 {
		return 0, errBadPaging
	}
	return int32(n), nil
}

// timeIDCursor is a keyset position on (created_at DESC, id DESC). It is
// encoded opaquely so clients treat it as a token, not a field to construct.
type timeIDCursor struct {
	CreatedAt time.Time
	ID        int64
}

func (c timeIDCursor) encode() string {
	raw := fmt.Sprintf("%s|%d", c.CreatedAt.UTC().Format(time.RFC3339Nano), c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// parseCursor reads ?cursor=; ok is false when no cursor was given.
func parseCursor(q url.Values) (c timeIDCursor, ok bool, err error) {
	v := q.Get("cursor")
	if v == "" {
		return timeIDCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return timeIDCursor{}, false, errBadPaging
	}
	ts, idStr, found := strings.Cut(string(raw), "|")
	if !found {
		return timeIDCursor{}, false, errBadPaging
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return timeIDCursor{}, false, errBadPaging
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return timeIDCursor{}, false, errBadPaging
	}
	return timeIDCursor{CreatedAt: t, ID: id}, true, nil
}

// likePrefix builds a LIKE "starts with" pattern, escaping the LIKE wildcards
// and the escape character so the prefix matches literally.
func likePrefix(term string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(term) + "%"
}
