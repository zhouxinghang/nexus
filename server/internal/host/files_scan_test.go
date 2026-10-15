package host

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestWalkSkipsDotPrefixedEntries(t *testing.T) {
	for name, fs := range map[string]FS{
		"real": NewRealHost(),
		"fake": NewFakeHost(),
	} {
		t.Run(name, func(t *testing.T) {
			// A dot-prefixed ancestor and the explicitly selected Managed Root
			// must not hide otherwise visible descendants.
			root := filepath.Join(t.TempDir(), ".ancestor", ".root")
			want := []string{"a.txt", filepath.Join("sub", "b.txt"), "with.dot.txt"}
			hidden := []string{
				".env",
				".DS_Store",
				"..hidden",
				filepath.Join("sub", ".secret"),
				filepath.Join(".git", "config"),
				filepath.Join("sub", ".cache", "nested", "data.txt"),
			}
			for _, rel := range append(append([]string{}, want...), hidden...) {
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			items, err := fs.Walk(root + string(filepath.Separator))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, item := range items {
				rel, err := filepath.Rel(root, item.Path)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, rel)
			}
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Walk = %v, want %v", got, want)
			}
			dirs, err := fs.ListWithinRoot(root, root)
			if err != nil || len(dirs) != 1 || dirs[0].Name != "sub" {
				t.Fatalf("direct directories = %+v, err=%v", dirs, err)
			}
			dirs, err = fs.ListWithinRoot(root, filepath.Join(root, "sub"))
			if err != nil || len(dirs) != 0 {
				t.Fatalf("hidden nested directories = %+v, err=%v", dirs, err)
			}
		})
	}
}
