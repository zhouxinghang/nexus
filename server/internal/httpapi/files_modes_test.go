package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFolderModeReturnsEveryDirectResourceWithoutPaging(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	root := h.createRoot(t, dir)
	// More than both the normal page size and the recursive endpoint's cap.
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO resources (root_id, path, name, size)
		SELECT $1, $2 || '/item-' || lpad(n::text, 4, '0') || '.txt',
		       'item-' || lpad(n::text, 4, '0') || '.txt', n
		FROM generate_series(1, 1205) n`, root.ID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(context.Background(), `
		INSERT INTO resources (root_id, path, name, size)
		VALUES ($1, $2 || '/nested/deep.txt', 'deep.txt', 9999)`, root.ID, dir); err != nil {
		t.Fatal(err)
	}
	page := h.resourcePage(browseQuery(root.ID, dir) + "&mode=folder&sort=size&order=desc")
	if len(page.Items) != 1205 || page.Total != 1205 || page.Limit != 0 || page.Offset != 0 {
		t.Fatalf("folder was paginated: count=%d total=%d limit=%d offset=%d", len(page.Items), page.Total, page.Limit, page.Offset)
	}
	if page.Items[0].Name != "item-1205.txt" || page.Items[1204].Name != "item-0001.txt" || page.Kinds["document"] != 1205 {
		t.Fatalf("folder sorting or counts incorrect")
	}
	if len(page.Directories) != 1 || page.Directories[0].Name != "nested" {
		t.Fatalf("nested directory not navigable: %+v", page.Directories)
	}
	paged := h.resourcePage(fmt.Sprintf("?mode=type&root_id=%d&limit=9999", root.ID))
	if len(paged.Items) != 1000 || paged.Limit != 1000 || paged.Total != 1206 {
		t.Fatalf("type mode escaped its page cap: count=%d limit=%d total=%d", len(paged.Items), paged.Limit, paged.Total)
	}
}

func TestTypeModeCombinesLiteralNameFolderTagAndKindFilters(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	root := h.createRoot(t, dir)
	ctx := context.Background()
	for _, item := range []struct {
		path string
		name string
		tag  string
	}{
		{"docs/资料_100%\\FINAL.txt", "资料_100%\\FINAL.txt", "keep"},
		{"docs/deep/资料_100%\\final.txt", "资料_100%\\final.txt", "keep"},
		{"docs/资料_100%\\final.png", "资料_100%\\final.png", "keep"},
		{"docs-extra/资料_100%\\final.txt", "资料_100%\\final.txt", "keep"},
		{"docs/资料X100anythingFINAL.txt", "资料X100anythingFINAL.txt", "keep"},
		{"docs/资料_100%\\untagged.txt", "资料_100%\\untagged.txt", "other"},
	} {
		var id int64
		if err := h.pool.QueryRow(ctx, `
			INSERT INTO resources (root_id,path,name)
			VALUES ($1,$2,$3) RETURNING id`, root.ID, filepath.Join(dir, item.path), item.name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Exec(ctx, "INSERT INTO resource_tags (resource_id,tag) VALUES ($1,$2)", id, item.tag); err != nil {
			t.Fatal(err)
		}
	}
	q := url.Values{
		"mode": {"type"}, "dir": {filepath.Join(dir, "docs")},
		"name": {`资料_100%\final`}, "tag": {"keep"}, "kind": {"document"},
		"limit": {"1"}, "sort": {"name"}, "order": {"asc"},
	}
	// No root_id: the type view must work across every Managed Root.
	page := h.resourcePage("?" + q.Encode())
	if page.Total != 2 || len(page.Items) != 1 || page.Kinds["document"] != 2 || page.Kinds["image"] != 1 || len(page.Directories) != 0 {
		t.Fatalf("combined filter/count mismatch: %+v", page)
	}
	q.Set("offset", "1")
	next := h.resourcePage("?" + q.Encode())
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.Total != 2 {
		t.Fatalf("second server page incorrect: %+v", next)
	}
	if !reflect.DeepEqual(next.Items[0].Tags, []string{"keep"}) {
		t.Fatalf("tags lost during paging: %+v", next.Items[0])
	}
}

func TestFolderModeCannotDisablePagingForRecursiveQueries(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	root := h.createRoot(t, dir)
	for _, query := range []string{
		"?mode=unknown",
		"?mode=folder",
		fmt.Sprintf("?mode=folder&root_id=%d", root.ID),
		"?mode=folder&parent=" + url.QueryEscape(dir),
		browseQuery(root.ID, dir) + "&mode=folder&limit=1",
		browseQuery(root.ID, dir) + "&mode=folder&offset=0",
		browseQuery(root.ID, dir) + "&mode=folder&tag=keep",
		browseQuery(root.ID, dir) + "&mode=folder&kind=image",
		browseQuery(root.ID, dir) + "&mode=folder&name=foo",
		browseQuery(root.ID, dir) + "&mode=type",
		"?mode=type&dir=relative",
	} {
		t.Run(query, func(t *testing.T) {
			resp := h.do(http.MethodGet, "/api/files/resources"+query, nil)
			mustStatus(t, resp, http.StatusBadRequest)
			resp.Body.Close()
		})
	}
}
