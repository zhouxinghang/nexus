package host

import (
	"os"
	"path/filepath"
	"strings"
)

// IsIgnoredPath reports whether a descendant of root has a dot-prefixed name.
// The explicitly configured Managed Root and its ancestors are not excluded.
func IsIgnoredPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(name, ".") {
			return true
		}
	}
	return false
}

// walkFiles excludes hidden entries before reading their metadata. Hidden
// directories are pruned, so none of their descendants reach the index.
func walkFiles(root string) ([]FileInfo, error) {
	root = filepath.Clean(root)
	var out []FileInfo
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil // vanished between the walk and the stat
		}
		out = append(out, toFileInfo(path, fi))
		return nil
	})
	return out, err
}
