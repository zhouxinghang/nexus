package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListWithinRootIncludesOnlyDirectDirectories(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	items, err := listWithinRoot(root, root)
	if err != nil || len(items) != 2 {
		t.Fatalf("direct directories = %+v, err=%v", items, err)
	}
	for _, item := range items {
		if !item.IsDir || filepath.Dir(item.Path) != root || (item.Name != "sub" && item.Name != "empty") {
			t.Fatalf("unexpected child: %+v", item)
		}
	}
	items, err = listWithinRoot(root, sub)
	if err != nil || len(items) != 1 || items[0].Name != "nested" {
		t.Fatalf("nested directories = %+v, err=%v", items, err)
	}
	items, err = listWithinRoot(root, filepath.Join(root, "empty"))
	if err != nil || len(items) != 0 {
		t.Fatalf("empty directory = %+v, err=%v", items, err)
	}
	for _, path := range []string{filepath.Dir(root), filepath.Join(root, "outside")} {
		if _, err := listWithinRoot(root, path); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("escape %q returned %v", path, err)
		}
	}
}
