package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type directoryResp struct {
	Path       string     `json:"path"`
	Name       string     `json:"name"`
	ModifiedAt *time.Time `json:"modified_at"`
	State      string     `json:"state"`
}

func browseQuery(rootID int64, parent string) string {
	return fmt.Sprintf("?root_id=%d&parent=%s", rootID, url.QueryEscape(parent))
}

func resourceNames(items []resourceResp) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Name
	}
	return out
}

func TestDirectoryBrowsingPreservesHierarchyAndEmptyDirectories(t *testing.T) {
	h := newHarness(t)
	// SQL wildcard characters and multibyte paths must be treated literally.
	dir := filepath.Join(t.TempDir(), `资料_100%\docs`)
	sub := filepath.Join(dir, "nested")
	deep := filepath.Join(sub, "deep")
	empty := filepath.Join(dir, "empty")
	for _, path := range []string{deep, empty} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "top.txt"), "top")
	writeFile(t, filepath.Join(sub, "child.txt"), "child")
	writeFile(t, filepath.Join(deep, "leaf.txt"), "leaf")
	root := h.createRoot(t, dir)
	all := h.scan(t, root.ID)
	if len(all) != 3 {
		t.Fatalf("recursive API changed: %+v", all)
	}
	// A neighboring Managed Root must not leak into directory navigation.
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "outside.txt"), "outside")
	otherRoot := h.createRoot(t, other)
	h.scan(t, otherRoot.ID)

	page := h.resourcePage(browseQuery(root.ID, dir))
	if !reflect.DeepEqual(resourceNames(page.Items), []string{"top.txt"}) || page.Total != 1 || page.Kinds["document"] != 1 {
		t.Fatalf("root directory was flattened: %+v", page)
	}
	if len(page.Directories) != 2 || page.Directories[0].Name != "empty" || page.Directories[1].Path != sub {
		t.Fatalf("direct directory children = %+v", page.Directories)
	}
	page = h.resourcePage(browseQuery(root.ID, sub))
	if !reflect.DeepEqual(resourceNames(page.Items), []string{"child.txt"}) || len(page.Directories) != 1 || page.Directories[0].Path != deep {
		t.Fatalf("nested directory = %+v", page)
	}
	page = h.resourcePage(browseQuery(root.ID, deep))
	if !reflect.DeepEqual(resourceNames(page.Items), []string{"leaf.txt"}) || len(page.Directories) != 0 {
		t.Fatalf("leaf directory = %+v", page)
	}
	page = h.resourcePage(browseQuery(root.ID, empty))
	if page.Total != 0 || len(page.Items) != 0 || len(page.Directories) != 0 {
		t.Fatalf("empty directory = %+v", page)
	}
	// Filters affect direct Resources, never hide navigation to descendants.
	page = h.resourcePage(browseQuery(root.ID, dir) + "&tag=none&kind=image")
	if page.Total != 0 || len(page.Directories) != 2 {
		t.Fatalf("filters hid folders: %+v", page)
	}
}

func TestMissingDirectoryResourcesRemainReachableWithTags(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "gone", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "missing.txt"), "x")
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	tagResp := h.do(http.MethodPost, path("/api/files/resources/", id)+"/tags", map[string]string{"tag": "keep"})
	mustStatus(t, tagResp, http.StatusOK)
	tagResp.Body.Close()
	if err := os.RemoveAll(filepath.Join(dir, "gone")); err != nil {
		t.Fatal(err)
	}
	h.scan(t, root.ID)
	for _, parent := range []string{dir, filepath.Join(dir, "gone")} {
		page := h.resourcePage(browseQuery(root.ID, parent))
		if len(page.Directories) != 1 || page.Directories[0].State != "missing" {
			t.Fatalf("missing ancestor under %q = %+v", parent, page)
		}
	}
	page := h.resourcePage(browseQuery(root.ID, sub) + "&tag=keep")
	if page.Total != 1 || page.Items[0].State != "missing" || !reflect.DeepEqual(page.Items[0].Tags, []string{"keep"}) {
		t.Fatalf("Missing Resource/tags inaccessible: %+v", page)
	}
}

func TestDirectoryBrowsingRejectsEscapesAndSymlinks(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	root := h.createRoot(t, dir)
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if page := h.resourcePage(browseQuery(root.ID, dir)); len(page.Directories) != 0 {
		t.Fatalf("outside symlink disclosed: %+v", page)
	}
	for _, query := range []string{
		browseQuery(root.ID, outside),
		browseQuery(root.ID, filepath.Join(dir, "..")),
		browseQuery(root.ID, "relative"),
		browseQuery(root.ID, link),
		"?parent=" + url.QueryEscape(dir),
		browseQuery(root.ID, dir) + "&dir=" + url.QueryEscape(dir),
	} {
		resp := h.do(http.MethodGet, "/api/files/resources"+query, nil)
		mustStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
}

func TestResourceSortBeforePaginationWithStableTies(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, file := range []struct {
		name string
		size int
		age  time.Duration
	}{
		{"a.txt", 30, 2 * time.Hour},
		{"b.txt", 10, time.Hour},
		{"c.txt", 10, time.Hour},
		{"d.txt", 20, 3 * time.Hour},
	} {
		path := filepath.Join(dir, file.name)
		writeFile(t, path, strings.Repeat("x", file.size))
		if err := os.Chtimes(path, now.Add(-file.age), now.Add(-file.age)); err != nil {
			t.Fatal(err)
		}
	}
	root := h.createRoot(t, dir)
	all := h.scan(t, root.ID)
	for _, item := range all {
		resp := h.do(http.MethodPost, path("/api/files/resources/", item.ID)+"/tags", map[string]string{"tag": "keep"})
		mustStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}
	cases := []struct {
		query string
		names []string
	}{
		{"sort=size&order=asc", []string{"b.txt", "c.txt", "d.txt", "a.txt"}},
		{"sort=size&order=desc", []string{"a.txt", "d.txt", "b.txt", "c.txt"}},
		{"sort=modified&order=asc", []string{"d.txt", "a.txt", "b.txt", "c.txt"}},
		{"sort=modified&order=desc", []string{"b.txt", "c.txt", "a.txt", "d.txt"}},
		{"sort=name&order=desc", []string{"d.txt", "c.txt", "b.txt", "a.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			var got []string
			for offset := 0; offset < 4; offset += 2 {
				page := h.resourcePage(browseQuery(root.ID, dir) + "&tag=keep&kind=document&" + tc.query + fmt.Sprintf("&limit=2&offset=%d", offset))
				if page.Total != 4 || page.Kinds["document"] != 4 {
					t.Fatalf("sorting changed counts: %+v", page)
				}
				got = append(got, resourceNames(page.Items)...)
			}
			if !reflect.DeepEqual(got, tc.names) {
				t.Fatalf("paged order = %v, want %v", got, tc.names)
			}
		})
	}
	// A missing modification time always sorts last, in either direction.
	if _, err := h.pool.Exec(context.Background(), "UPDATE resources SET modified_at = NULL WHERE name = 'a.txt'"); err != nil {
		t.Fatal(err)
	}
	for _, order := range []string{"asc", "desc"} {
		page := h.resourcePage(browseQuery(root.ID, dir) + "&sort=modified&order=" + order)
		if page.Items[len(page.Items)-1].Name != "a.txt" {
			t.Fatalf("null modification time not last: %+v", page.Items)
		}
	}
	for _, query := range []string{"?sort=bogus", "?order=bogus", "?sort=size%3BDROP+TABLE+resources", "?sort=path&order=desc"} {
		resp := h.do(http.MethodGet, "/api/files/resources"+query, nil)
		mustStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
}

func TestDropNotesMigrationPreservesResourceTags(t *testing.T) {
	var id int64
	h := newHarnessSeeded(t, 16, func(ctx context.Context, pool *pgxpool.Pool) {
		var rootID int64
		if err := pool.QueryRow(ctx, "INSERT INTO managed_roots (path) VALUES ('/old') RETURNING id").Scan(&rootID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "INSERT INTO resources (root_id,path,name,note) VALUES ($1,'/old/a.txt','a.txt','legacy note') RETURNING id", rootID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO resource_tags (resource_id,tag) VALUES ($1,'keep')", id); err != nil {
			t.Fatal(err)
		}
	})
	resp := h.do(http.MethodGet, path("/api/files/resources/", id), nil)
	mustStatus(t, resp, http.StatusOK)
	var resource resourceResp
	decode(t, resp, &resource)
	if resource.ID != id || resource.Path != "/old/a.txt" || !reflect.DeepEqual(resource.Tags, []string{"keep"}) {
		t.Fatalf("migration changed Resource: %+v", resource)
	}
	var count int
	if err := h.pool.QueryRow(context.Background(), "SELECT count(*) FROM information_schema.columns WHERE table_name='resources' AND column_name='note'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("note column still exists: count=%d err=%v", count, err)
	}
}
