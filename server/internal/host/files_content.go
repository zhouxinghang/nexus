package host

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxTextFileSize = 2 << 20 // 2 MiB, including a possible UTF-8 BOM

var (
	ErrUnsafePath  = errors.New("path cannot be accessed within the Managed Root")
	ErrNotRegular  = errors.New("only regular files can be previewed or edited")
	ErrFileChanged = errors.New("file changed on disk; reload before saving")
	ErrTooLarge    = errors.New("text files larger than 2 MiB cannot be edited")
	ErrSymlink     = errors.New("symbolic links are read-only")
)

// ContentRevision identifies the actual bytes, not the potentially stale index
// or a coarse filesystem timestamp. Two same-size edits still conflict.
func ContentRevision(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// resourceParent pins the parent directory within the Managed Root. os.Root
// confines symlink resolution (including intermediate directories) in the
// kernel-backed filesystem seam, rather than trusting a lexical path check.
func resourceParent(rootPath, path string) (*os.Root, string, error) {
	rel, err := filepath.Rel(rootPath, path)
	if err != nil || filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", ErrUnsafePath
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	parent, err := root.OpenRoot(filepath.Dir(rel))
	if err != nil {
		return nil, "", safePathError(err)
	}
	return parent, filepath.Base(rel), nil
}

func safePathError(err error) error {
	if os.IsNotExist(err) || os.IsPermission(err) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUnsafePath, err)
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	// Reject a directory, device or FIFO before opening it; a FIFO would block.
	fi, err := root.Stat(name)
	if err != nil {
		return nil, safePathError(err)
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, safePathError(err)
	}
	fi, err = f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrNotRegular
	}
	return f, nil
}

func openWithinRoot(rootPath, path string) (io.ReadSeekCloser, FileInfo, bool, error) {
	parent, name, err := resourceParent(rootPath, path)
	if err != nil {
		return nil, FileInfo{}, false, err
	}
	defer parent.Close()
	linkInfo, err := parent.Lstat(name)
	if err != nil {
		return nil, FileInfo{}, false, err
	}
	f, err := openRegular(parent, name)
	if err != nil {
		return nil, FileInfo{}, false, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, FileInfo{}, false, err
	}
	editable := linkInfo.Mode()&os.ModeSymlink == 0 && fi.Mode().Perm()&0o222 != 0
	return f, toFileInfo(path, fi), editable, nil
}

// writeText replaces the bytes atomically in the same directory, preserving
// permissions. The caller serializes Nexus saves; external writes are checked
// both before staging and immediately before rename. Filesystem writes cannot
// participate in a Postgres transaction.
func writeText(rootPath, path string, data []byte, revision string) (FileInfo, error) {
	if len(data) > MaxTextFileSize {
		return FileInfo{}, ErrTooLarge
	}
	parent, name, err := resourceParent(rootPath, path)
	if err != nil {
		return FileInfo{}, err
	}
	defer parent.Close()
	check := func() (os.FileInfo, error) {
		fi, err := parent.Lstat(name)
		if err != nil {
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, ErrSymlink
		}
		if !fi.Mode().IsRegular() {
			return nil, ErrNotRegular
		}
		if fi.Mode().Perm()&0o222 == 0 {
			return nil, os.ErrPermission
		}
		f, err := openRegular(parent, name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		current, err := io.ReadAll(io.LimitReader(f, MaxTextFileSize+1))
		if err != nil {
			return nil, err
		}
		if len(current) > MaxTextFileSize || ContentRevision(current) != revision {
			return nil, ErrFileChanged
		}
		return fi, nil
	}
	fi, err := check()
	if err != nil {
		return FileInfo{}, err
	}
	temp := ".nexus-edit-" + rand.Text()
	f, err := parent.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return FileInfo{}, err
	}
	defer parent.Remove(temp)
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(fi.Mode().Perm())
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return FileInfo{}, err
	}
	if closeErr != nil {
		return FileInfo{}, closeErr
	}
	if _, err := check(); err != nil {
		return FileInfo{}, err
	}
	if err := parent.Rename(temp, name); err != nil {
		return FileInfo{}, err
	}
	fi, err = parent.Stat(name)
	if err != nil {
		return FileInfo{}, err
	}
	return toFileInfo(path, fi), nil
}
