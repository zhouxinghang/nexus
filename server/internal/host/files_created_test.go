package host

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCreationTimeIsNotModificationTime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "resource.txt")
	before := time.Now().UTC().Truncate(time.Microsecond)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewRealHost()
	first, err := h.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.CreatedAt == nil {
		t.Skip("filesystem creation time is unavailable")
	}
	if first.CreatedAt.Before(before) || first.CreatedAt.After(time.Now()) {
		t.Fatalf("creation time %v does not describe the newly created file", first.CreatedAt)
	}
	// Setting an earlier mtime can also move APFS's birth time backwards.
	// A later mtime lets us check that we read the independent birth field.
	modified := first.CreatedAt.Add(24 * time.Hour)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	for name, fs := range map[string]FS{"real": h, "fake": NewFakeHost()} {
		t.Run(name, func(t *testing.T) {
			items, err := fs.Walk(root)
			if err != nil || len(items) != 1 {
				t.Fatalf("Walk = %+v, err=%v", items, err)
			}
			if items[0].CreatedAt == nil || !items[0].CreatedAt.Equal(*first.CreatedAt) {
				t.Fatalf("creation time changed with mtime: %+v", items[0])
			}
			if !items[0].ModifiedAt.Equal(modified) {
				t.Fatalf("mtime = %v, want %v", items[0].ModifiedAt, modified)
			}
		})
	}
}
