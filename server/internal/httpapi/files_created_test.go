package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFileCreationMetadataScanRegisterAndBackfill(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "resource.txt")
	writeFile(t, file, "before")
	info, err := h.host.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.CreatedAt == nil {
		t.Skip("filesystem creation time is unavailable")
	}
	modified := info.CreatedAt.Add(24 * time.Hour)
	if err := os.Chtimes(file, modified, modified); err != nil {
		t.Fatal(err)
	}
	root := h.createRoot(t, dir)
	registered := h.do(http.MethodPost, "/api/files/resources/register", map[string]string{"path": file})
	mustStatus(t, registered, http.StatusCreated)
	var resource resourceResp
	decode(t, registered, &resource)
	if resource.FileCreatedAt == nil || !resource.FileCreatedAt.Equal(*info.CreatedAt) {
		t.Fatalf("registered creation metadata = %+v, want %v", resource, info.CreatedAt)
	}
	tag := h.do(http.MethodPost, path("/api/files/resources/", resource.ID)+"/tags", map[string]string{"tag": "keep"})
	mustStatus(t, tag, http.StatusOK)
	tag.Body.Close()

	// Emulate a pre-migration row. Its size and mtime still match, but Scan
	// must backfill the missing creation time without replacing id or tags.
	if _, err := h.pool.Exec(context.Background(), "UPDATE resources SET file_created_at=NULL WHERE id=$1", resource.ID); err != nil {
		t.Fatal(err)
	}
	st := h.scanAndWait(t, root.ID)
	if st.Indexed != 1 {
		t.Fatalf("unchanged legacy Resource was not backfilled: %+v", st)
	}
	page := h.resourcePage("?sort=created&order=asc")
	if len(page.Items) != 1 || page.Items[0].ID != resource.ID || !reflect.DeepEqual(page.Items[0].Tags, []string{"keep"}) {
		t.Fatalf("backfill changed identity/tags: %+v", page)
	}
	if page.Items[0].FileCreatedAt == nil || !page.Items[0].FileCreatedAt.Equal(*info.CreatedAt) {
		t.Fatalf("backfilled creation time = %v, want %v", page.Items[0].FileCreatedAt, info.CreatedAt)
	}
	if st = h.scanAndWait(t, root.ID); st.Indexed != 0 {
		t.Fatalf("backfilled Resource was rewritten on another scan: %+v", st)
	}

	var content textContentResp
	decode(t, h.do(http.MethodGet, path("/api/files/resources/", resource.ID)+"/content", nil), &content)
	saved := h.do(http.MethodPut, path("/api/files/resources/", resource.ID)+"/content", map[string]string{
		"content": "after", "revision": content.Revision,
	})
	mustStatus(t, saved, http.StatusOK)
	saved.Body.Close()
	info, err = h.host.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, h.do(http.MethodGet, path("/api/files/resources/", resource.ID), nil), &resource)
	if resource.FileCreatedAt == nil || info.CreatedAt == nil || !resource.FileCreatedAt.Equal(*info.CreatedAt) {
		t.Fatalf("atomic edit did not refresh creation time: index=%v, disk=%v", resource.FileCreatedAt, info.CreatedAt)
	}
}

func TestResourceCreationSortBeforePaginationWithUnknownsLast(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)
	if _, err := h.pool.Exec(context.Background(), `
		UPDATE resources SET
		  file_created_at = CASE name
		    WHEN 'a.txt' THEN '2001-01-01'::timestamptz
		    WHEN 'b.txt' THEN '2002-01-01'::timestamptz
		    WHEN 'c.txt' THEN '2002-01-01'::timestamptz END,
		  created_at = CASE name
		    WHEN 'a.txt' THEN '2020-01-04'::timestamptz
		    WHEN 'b.txt' THEN '2020-01-03'::timestamptz
		    WHEN 'c.txt' THEN '2020-01-02'::timestamptz
		    ELSE '2020-01-01'::timestamptz END
		WHERE root_id=$1`, root.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		order string
		names []string
	}{
		{"asc", []string{"a.txt", "b.txt", "c.txt", "d.txt"}},
		{"desc", []string{"b.txt", "c.txt", "a.txt", "d.txt"}},
	} {
		t.Run(tc.order, func(t *testing.T) {
			var got []string
			for offset := 0; offset < 4; offset += 2 {
				page := h.resourcePage(fmt.Sprintf("?mode=type&root_id=%d&kind=document&sort=created&order=%s&limit=2&offset=%d", root.ID, tc.order, offset))
				if page.Total != 4 || page.Kinds["document"] != 4 {
					t.Fatalf("creation sorting changed totals: %+v", page)
				}
				got = append(got, resourceNames(page.Items)...)
			}
			if !reflect.DeepEqual(got, tc.names) {
				t.Fatalf("creation order = %v, want %v", got, tc.names)
			}
			page := h.resourcePage(browseQuery(root.ID, dir) + "&mode=folder&sort=created&order=" + tc.order)
			if !reflect.DeepEqual(resourceNames(page.Items), tc.names) || page.Items[3].FileCreatedAt != nil {
				t.Fatalf("folder creation order/unknown time = %+v", page)
			}
		})
	}
}

func TestDirectoryCreationSortKeepsMissingDirectoriesLast(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	older := filepath.Join(dir, "z-older")
	newer := filepath.Join(dir, "a-newer")
	if err := os.Mkdir(older, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.Mkdir(newer, 0o755); err != nil {
		t.Fatal(err)
	}
	fi, err := h.host.Stat(older)
	if err != nil {
		t.Fatal(err)
	}
	if fi.CreatedAt == nil {
		t.Skip("filesystem creation time is unavailable")
	}
	missing := filepath.Join(dir, "missing")
	writeFile(t, filepath.Join(missing, "gone.txt"), "x")
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	h.scanAndWait(t, root.ID)
	for _, tc := range []struct {
		order string
		names []string
	}{
		{"asc", []string{"z-older", "a-newer", "missing"}},
		{"desc", []string{"a-newer", "z-older", "missing"}},
	} {
		page := h.resourcePage(browseQuery(root.ID, dir) + "&sort=created&order=" + tc.order)
		var names []string
		for _, item := range page.Directories {
			names = append(names, item.Name)
		}
		if !reflect.DeepEqual(names, tc.names) || page.Directories[2].State != "missing" {
			t.Fatalf("directory creation order = %+v, want %v", page.Directories, tc.names)
		}
	}
}
