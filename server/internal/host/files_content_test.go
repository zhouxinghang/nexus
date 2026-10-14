package host

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentWriteAtomicAndPreservesPermissions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "script.sh")
	before := "#!/bin/sh\r\necho 你好\r\n"
	after := "\uFEFF#!/bin/sh\r\necho updated\r\n"
	if err := os.WriteFile(path, []byte(before), 0o750); err != nil {
		t.Fatal(err)
	}
	h := NewFakeHost()
	fi, err := h.WriteText(root, path, []byte(after), ContentRevision([]byte(before)))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != after || fi.Size != int64(len([]byte(after))) {
		t.Fatalf("wrong saved content/metadata: %q, %+v", data, fi)
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0o750 {
		t.Fatalf("permissions = %o, want 750", stat.Mode().Perm())
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("temporary edit file left behind: %v", entries)
	}
}

func TestContentWriteRejectsSameSizeSameTimestampConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	os.WriteFile(path, []byte("old"), 0o644)
	fi, _ := os.Stat(path)
	os.WriteFile(path, []byte("new"), 0o644)
	os.Chtimes(path, fi.ModTime(), fi.ModTime())
	_, err := NewFakeHost().WriteText(root, path, []byte("my draft"), ContentRevision([]byte("old")))
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("error = %v, want conflict", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Fatal("external edit overwritten")
	}
}

func TestContentConfinementAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	os.WriteFile(target, []byte("secret"), 0o644)
	h := NewFakeHost()
	for name, path := range map[string]string{
		"direct":           target,
		"parent traversal": filepath.Join(root, "..", filepath.Base(outside), "secret.txt"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := h.OpenWithinRoot(root, path); err == nil {
				t.Fatal("out-of-root read allowed")
			}
			if _, err := h.WriteText(root, path, []byte("bad"), ContentRevision([]byte("secret"))); err == nil {
				t.Fatal("out-of-root write allowed")
			}
		})
	}
	escape := filepath.Join(root, "escape.txt")
	os.Symlink(target, escape)
	if _, _, _, err := h.OpenWithinRoot(root, escape); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink escape: %v", err)
	}
	os.Symlink(outside, filepath.Join(root, "outside"))
	if _, _, _, err := h.OpenWithinRoot(root, filepath.Join(root, "outside", "secret.txt")); err == nil {
		t.Fatal("intermediate symlink escaped the root")
	}
	inside := filepath.Join(root, "a.txt")
	os.WriteFile(inside, []byte("inside"), 0o644)
	link := filepath.Join(root, "link.txt")
	os.Symlink("a.txt", link)
	f, _, editable, err := h.OpenWithinRoot(root, link)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "inside" || editable {
		t.Fatalf("internal symlink should be readable, not editable: %q, %v", data, editable)
	}
	if _, err := h.WriteText(root, link, []byte("replacement"), ContentRevision(data)); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlink write = %v, want rejected", err)
	}
}

func TestContentRejectsDirectoryMissingAndOversized(t *testing.T) {
	root := t.TempDir()
	h := NewFakeHost()
	sub := filepath.Join(root, "sub")
	os.Mkdir(sub, 0o755)
	if _, _, _, err := h.OpenWithinRoot(root, sub); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory read = %v", err)
	}
	path := filepath.Join(root, "missing.txt")
	if _, err := h.WriteText(root, path, nil, ContentRevision(nil)); !os.IsNotExist(err) {
		t.Fatalf("missing write = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("editing resurrected missing file")
	}
	os.WriteFile(path, []byte("original"), 0o644)
	if _, err := h.WriteText(root, path, []byte(strings.Repeat("x", MaxTextFileSize+1)), ContentRevision([]byte("original"))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized write = %v", err)
	}
}
