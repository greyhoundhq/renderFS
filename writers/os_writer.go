package writers

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/greyhoundhq/renderfs"
)

// OSWriter implements renderfs.Writer for the local filesystem rooted at DestDir.
type OSWriter struct {
	DestDir string
}

// NewOSWriter constructs an OSWriter rooted at destDir. The destination path
// is resolved to an absolute path to prevent directory traversal.
func NewOSWriter(destDir string) (*OSWriter, error) {
	abs, err := filepath.Abs(destDir)
	if err != nil {
		return nil, err
	}
	return &OSWriter{DestDir: abs}, nil
}

func (w *OSWriter) join(path string) string {
	return filepath.Join(w.DestDir, filepath.FromSlash(path))
}

// MkdirAll creates directories on disk and ensures the final directory has the
// requested permissions.
func (w *OSWriter) MkdirAll(path string, perm fs.FileMode) error {
	full := w.join(path)
	if err := os.MkdirAll(full, perm); err != nil {
		return err
	}
	return os.Chmod(full, perm.Perm())
}

// CreateFile opens a file for writing, creating any missing parent directories.
func (w *OSWriter) CreateFile(path string, perm fs.FileMode) (io.WriteCloser, error) {
	full := w.join(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(full, perm.Perm()); err != nil {
		_ = f.Close()
		return nil, err
	}

	return f, nil
}

// Open opens the named file for reading.
func (w *OSWriter) Open(path string) (io.ReadCloser, error) {
	return os.Open(w.join(path))
}

// Symlink creates a symbolic link within DestDir.
func (w *OSWriter) Symlink(oldname, newname string) error {
	full := w.join(newname)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.Symlink(oldname, full)
}

// Lstat reports information about a path relative to DestDir. It allows Copy to
// implement conflict handling semantics.
func (w *OSWriter) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(w.join(path))
}

var _ renderfs.Writer = (*OSWriter)(nil)
