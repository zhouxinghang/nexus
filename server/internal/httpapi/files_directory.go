package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

type directoryResponse struct {
	Path       string     `json:"path"`
	Name       string     `json:"name"`
	ModifiedAt *time.Time `json:"modified_at"`
	State      string     `json:"state"`
}

func (s *Server) resourceDirectories(r *http.Request, root dbgen.ManagedRoot, parent, sortBy, order string) ([]directoryResponse, error) {
	live, err := s.host.ListWithinRoot(root.Path, parent)
	// Missing directory trees retain their indexed Resources and tags.
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	prefix := strings.TrimRight(parent, string(filepath.Separator)) + string(filepath.Separator)
	indexed, err := s.queries.ListResourceDirectories(r.Context(), dbgen.ListResourceDirectoriesParams{
		RootID: root.ID, ParentPrefix: prefix, DirPattern: likePrefix(prefix),
	})
	if err != nil {
		return nil, err
	}
	dirs := map[string]directoryResponse{}
	for _, path := range indexed {
		dirs[path] = directoryResponse{Path: path, Name: filepath.Base(path), State: "missing"}
	}
	for _, fi := range live {
		mod := fi.ModifiedAt
		dirs[fi.Path] = directoryResponse{Path: fi.Path, Name: fi.Name, ModifiedAt: &mod, State: "present"}
	}
	out := make([]directoryResponse, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, dir)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if sortBy == "modified" {
			if a.ModifiedAt == nil || b.ModifiedAt == nil {
				if (a.ModifiedAt == nil) != (b.ModifiedAt == nil) {
					return b.ModifiedAt == nil // nulls last in either direction
				}
			} else if !a.ModifiedAt.Equal(*b.ModifiedAt) {
				if order == "desc" {
					return a.ModifiedAt.After(*b.ModifiedAt)
				}
				return a.ModifiedAt.Before(*b.ModifiedAt)
			}
		}
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if sortBy == "name" && order == "desc" && an != bn {
			return an > bn
		}
		if an != bn {
			return an < bn
		}
		return a.Path < b.Path
	})
	return out, nil
}

func writeDirectoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, host.ErrUnsafePath):
		writeError(w, http.StatusBadRequest, "directory cannot be accessed within the Managed Root")
	case os.IsPermission(err):
		writeError(w, http.StatusForbidden, "directory is not readable")
	default:
		writeError(w, http.StatusInternalServerError, "could not list directories")
	}
}
