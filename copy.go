package renderfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Copy walks the source filesystem, renders templates, and writes to dest.
// It returns statistics on files created, updated, skipped, or found identical.
func Copy(source fs.FS, dest Writer, opts Options) (Stats, error) {
	var stats Stats

	if source == nil {
		return stats, fmt.Errorf("renderfs: source filesystem is required")
	}
	if dest == nil {
		return stats, fmt.Errorf("renderfs: destination writer is required")
	}

	context := opts.Context
	if context == nil {
		context = map[string]any{}
	}
	env := opts.Environment

	conflict := opts.OnConflict
	if conflict < Overwrite || conflict > Fail {
		conflict = Overwrite
	}

	matcher, err := buildIgnoreMatcher(source, opts.IgnorePatterns)
	if err != nil {
		return stats, err
	}

	err = fs.WalkDir(source, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if rel == "." {
			return nil
		}

		if matcher != nil && matcher.MatchesPath(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if rel == ".renderfs-ignore" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("renderfs: stat %s: %w", rel, err)
		}

		renderedRel, skip, err := RenderPathWithEnv(rel, d.IsDir(), context, opts.StrictVariables, env)
		if err != nil {
			return &RenderError{Kind: RenderErrorPath, Path: rel, Err: err}
		}
		if skip {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := readSymlink(source, rel)
			if err != nil {
				return fmt.Errorf("renderfs: read symlink %s: %w", rel, err)
			}
			if err := dest.Symlink(target, renderedRel); err != nil {
				return fmt.Errorf("renderfs: create symlink %s -> %s: %w", renderedRel, target, err)
			}
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return dest.MkdirAll(renderedRel, directoryMode(info))
		}

		rawContent, err := fs.ReadFile(source, rel)
		if err != nil {
			return fmt.Errorf("renderfs: read %s: %w", rel, err)
		}

		finalBytes, err := RenderBytesWithEnv(rawContent, context, opts.TemplateBinary, opts.StrictVariables, env)
		if err != nil {
			return &RenderError{Kind: RenderErrorFile, Path: rel, Err: err}
		}

		status, err := checkDestination(dest, renderedRel, finalBytes, conflict)
		if err != nil {
			return err
		}

		switch status {
		case statusIdentical:
			stats.Identical++
			return nil
		case statusSkip:
			stats.Skipped++
			return nil
		case statusUpdate:
			stats.Updated++
		case statusCreate:
			stats.Created++
		}

		if parent := path.Dir(renderedRel); parent != "." {
			if err := dest.MkdirAll(parent, 0o755); err != nil {
				return fmt.Errorf("renderfs: create parent %s: %w", parent, err)
			}
		}

		handle, err := dest.CreateFile(renderedRel, fileMode(info))
		if err != nil {
			return fmt.Errorf("renderfs: create %s: %w", renderedRel, err)
		}

		_, writeErr := handle.Write(finalBytes)
		closeErr := handle.Close()
		if writeErr != nil {
			return fmt.Errorf("renderfs: write %s: %w", renderedRel, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("renderfs: close %s: %w", renderedRel, closeErr)
		}

		return nil
	})

	return stats, err
}

type fileStatus int

const (
	statusCreate fileStatus = iota
	statusUpdate
	statusSkip
	statusIdentical
)

func checkDestination(dest Writer, path string, newContent []byte, conflict ConflictResolution) (fileStatus, error) {
	existing, err := dest.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return statusCreate, nil
		}
		return 0, fmt.Errorf("renderfs: check destination %s: %w", path, err)
	}
	defer existing.Close()

	oldContent, err := io.ReadAll(existing)
	if err != nil {
		return 0, fmt.Errorf("renderfs: read existing %s: %w", path, err)
	}

	if bytes.Equal(oldContent, newContent) {
		return statusIdentical, nil
	}

	switch conflict {
	case Skip:
		return statusSkip, nil
	case Fail:
		return 0, &RenderError{Kind: RenderErrorConflict, Path: path}
	case Overwrite:
		return statusUpdate, nil
	default:
		return statusUpdate, nil
	}
}

func stripTemplateSuffix(p string) string {
	switch {
	case strings.HasSuffix(p, ".jinja"):
		return strings.TrimSuffix(p, ".jinja")
	case strings.HasSuffix(p, ".tmpl"):
		return strings.TrimSuffix(p, ".tmpl")
	default:
		return p
	}
}

func directoryMode(info fs.FileInfo) fs.FileMode {
	perm := fs.FileMode(0o755)
	if info != nil {
		if p := info.Mode().Perm(); p != 0 {
			perm = p
		}
	}
	return perm
}

func fileMode(info fs.FileInfo) fs.FileMode {
	perm := fs.FileMode(0o644)
	if info != nil {
		if p := info.Mode().Perm(); p != 0 {
			perm = p
		}
	}
	return perm
}

func readSymlink(source fs.FS, rel string) (string, error) {
	if rl, ok := source.(fs.ReadLinkFS); ok {
		return rl.ReadLink(rel)
	}
	return "", fmt.Errorf("renderfs: source filesystem does not support symlinks")
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	mimeType := http.DetectContentType(sniff)
	return !strings.HasPrefix(mimeType, "text/") &&
		!strings.Contains(mimeType, "json") &&
		!strings.Contains(mimeType, "javascript") &&
		!strings.Contains(mimeType, "xml")
}
