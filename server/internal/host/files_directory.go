package host

import (
	"os"
	"path/filepath"
	"strings"
)

// listWithinRoot pins directory traversal to the Managed Root, including
// symlink resolution. Empty directories are browsable without indexing them
// as Resources or exposing recursive directory mutations.
func listWithinRoot(rootPath, path string) ([]FileInfo, error) {
	rel, err := filepath.Rel(rootPath, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrUnsafePath
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.OpenRoot(rel)
	if err != nil {
		return nil, safePathError(err)
	}
	defer dir.Close()
	f, err := dir.Open(".")
	if err != nil {
		return nil, safePathError(err)
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		// Symlinks are not directory navigation targets (Scan doesn't follow
		// them either). This also prevents outside-root directory disclosure.
		if !entry.IsDir() {
			continue
		}
		fi, err := entry.Info()
		if err == nil {
			out = append(out, toFileInfo(filepath.Join(path, entry.Name()), fi))
		}
	}
	return out, nil
}
