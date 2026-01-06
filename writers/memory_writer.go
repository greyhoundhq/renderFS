package writers

import (
	"bytes"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/greyhoundhq/renderfs"
)

// MemoryFile stores rendered file contents and metadata in memory.
type MemoryFile struct {
	Content *bytes.Buffer
	Mode    fs.FileMode
}

// MemorySymlink tracks symbolic links in memory.
type MemorySymlink struct {
	Target string
}

// MemoryWriter implements renderfs.Writer by storing output in memory. Useful
// for tests and dry-run previews.
type MemoryWriter struct {
	mu       sync.RWMutex
	files    map[string]*MemoryFile
	dirs     map[string]fs.FileMode
	symlinks map[string]*MemorySymlink
}

// NewMemoryWriter constructs a MemoryWriter instance.
func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{
		files:    make(map[string]*MemoryFile),
		dirs:     make(map[string]fs.FileMode),
		symlinks: make(map[string]*MemorySymlink),
	}
}

func normalizePath(p string) string {
	if p == "" || p == "." {
		return "."
	}
	clean := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if clean == "." {
		return "."
	}
	return clean
}

// MkdirAll records directory metadata. Directories are implicit, so we simply
// register the mode.
func (w *MemoryWriter) MkdirAll(p string, perm fs.FileMode) error {
	p = normalizePath(p)
	if p == "." {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.dirs[p] = perm
	return nil
}

// CreateFile stores file data in-memory, replacing any existing entry.
func (w *MemoryWriter) CreateFile(p string, perm fs.FileMode) (io.WriteCloser, error) {
	p = normalizePath(p)
	w.mu.Lock()
	defer w.mu.Unlock()

	file := &MemoryFile{
		Content: &bytes.Buffer{},
		Mode:    perm,
	}
	w.files[p] = file
	delete(w.symlinks, p)

	dir := path.Dir(p)
	if dir != "." {
		if _, ok := w.dirs[dir]; !ok {
			w.dirs[dir] = 0o755
		}
	}

	return &memoryFileWriteCloser{buf: file.Content}, nil
}

// Symlink records an in-memory symlink.
func (w *MemoryWriter) Symlink(oldname, newname string) error {
	newname = normalizePath(newname)
	w.mu.Lock()
	defer w.mu.Unlock()

	w.symlinks[newname] = &MemorySymlink{Target: oldname}
	delete(w.files, newname)

	dir := path.Dir(newname)
	if dir != "." {
		if _, ok := w.dirs[dir]; !ok {
			w.dirs[dir] = 0o755
		}
	}
	return nil
}

// Lstat reports metadata for conflict detection.
func (w *MemoryWriter) Lstat(p string) (fs.FileInfo, error) {
	p = normalizePath(p)

	w.mu.RLock()
	defer w.mu.RUnlock()

	if p == "." {
		return memoryDirInfo{name: ".", mode: 0o755 | fs.ModeDir}, nil
	}
	if dirMode, ok := w.dirs[p]; ok {
		return memoryDirInfo{name: path.Base(p), mode: dirMode | fs.ModeDir}, nil
	}
	if file, ok := w.files[p]; ok {
		return memoryFileInfo{name: path.Base(p), mode: file.Mode, size: int64(file.Content.Len())}, nil
	}
	if link, ok := w.symlinks[p]; ok {
		return memorySymlinkInfo{name: path.Base(p), target: link.Target}, nil
	}
	return nil, fs.ErrNotExist
}

// Contents returns a snapshot copy of the stored files for inspection.
func (w *MemoryWriter) Contents() map[string][]byte {
	w.mu.RLock()
	defer w.mu.RUnlock()

	out := make(map[string][]byte, len(w.files))
	for k, v := range w.files {
		out[k] = append([]byte(nil), v.Content.Bytes()...)
	}
	return out
}

// FileMode returns the stored mode for the file path.
func (w *MemoryWriter) FileMode(p string) (fs.FileMode, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if f, ok := w.files[normalizePath(p)]; ok {
		return f.Mode, true
	}
	return 0, false
}

// DirMode returns the stored mode for the directory path.
func (w *MemoryWriter) DirMode(p string) (fs.FileMode, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if mode, ok := w.dirs[normalizePath(p)]; ok {
		return mode, true
	}
	return 0, false
}

type memoryFileWriteCloser struct {
	buf *bytes.Buffer
}

func (wc *memoryFileWriteCloser) Write(p []byte) (int, error) {
	return wc.buf.Write(p)
}

func (wc *memoryFileWriteCloser) Close() error {
	return nil
}

type memoryFileInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (fi memoryFileInfo) Name() string       { return fi.name }
func (fi memoryFileInfo) Size() int64        { return fi.size }
func (fi memoryFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi memoryFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (fi memoryFileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi memoryFileInfo) Sys() interface{}   { return nil }

type memoryDirInfo struct {
	name string
	mode fs.FileMode
}

func (di memoryDirInfo) Name() string       { return di.name }
func (di memoryDirInfo) Size() int64        { return 0 }
func (di memoryDirInfo) Mode() fs.FileMode  { return di.mode }
func (di memoryDirInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (di memoryDirInfo) IsDir() bool        { return true }
func (di memoryDirInfo) Sys() interface{}   { return nil }

type memorySymlinkInfo struct {
	name   string
	target string
}

func (si memorySymlinkInfo) Name() string       { return si.name }
func (si memorySymlinkInfo) Size() int64        { return int64(len(si.target)) }
func (si memorySymlinkInfo) Mode() fs.FileMode  { return fs.ModeSymlink | 0o777 }
func (si memorySymlinkInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (si memorySymlinkInfo) IsDir() bool        { return false }
func (si memorySymlinkInfo) Sys() interface{}   { return nil }

var _ renderfs.Writer = (*MemoryWriter)(nil)
