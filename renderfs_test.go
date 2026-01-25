package renderfs_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/greyhoundhq/renderfs"
	"github.com/greyhoundhq/renderfs/writers"
)

func TestCopyBasicRendering(t *testing.T) {
	source := fstest.MapFS{
		"README.md.jinja": {
			Data: []byte("Project: {{ project_name }}\n"),
			Mode: 0o644,
		},
		"src": {
			Mode: fs.ModeDir | 0o755,
		},
		"src/{{ params.app_name }}": {
			Mode: fs.ModeDir | 0o755,
		},
		"src/{{ params.app_name }}/main.go.tmpl": {
			Data: []byte("package {{ params.app_name }}\n"),
			Mode: 0o755,
		},
	}

	context := map[string]any{
		"project_name": "RenderFS",
		"params": map[string]any{
			"app_name": "demo",
		},
	}

	writer := writers.NewMemoryWriter()

	stats, err := renderfs.Copy(source, writer, renderfs.Options{Context: context})
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if stats.Created != 2 {
		t.Fatalf("expected 2 created files, got %d", stats.Created)
	}

	contents := writer.Contents()
	if got := string(contents["README.md"]); got != "Project: RenderFS\n" {
		t.Fatalf("unexpected README content: %q", got)
	}

	mainPath := "src/demo/main.go"
	if got := string(contents[mainPath]); got != "package demo\n" {
		t.Fatalf("unexpected main.go content: %q", got)
	}

	mode, ok := writer.FileMode(mainPath)
	if !ok {
		t.Fatalf("expected mode for %s", mainPath)
	}
	if mode&0o755 != 0o755 {
		t.Fatalf("expected executable permissions, got %v", mode)
	}
}

func TestCopySkipsConditionalPath(t *testing.T) {
	source := fstest.MapFS{
		"{% if params.use_docker %}compose.yaml{% endif %}": {
			Data: []byte("version: '3.8'\n"),
		},
	}
	writer := writers.NewMemoryWriter()
	context := map[string]any{
		"params": map[string]any{
			"use_docker": false,
		},
	}

	stats, err := renderfs.Copy(source, writer, renderfs.Options{Context: context})
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if stats.Created != 0 {
		t.Fatalf("expected no created files, got %d", stats.Created)
	}

	if _, exists := writer.Contents()["compose.yaml"]; exists {
		t.Fatalf("expected compose.yaml to be skipped")
	}
}

func TestCopyRespectsIgnorePatterns(t *testing.T) {
	source := fstest.MapFS{
		".renderfs-ignore": {
			Data: []byte("ignored.txt\n"),
		},
		"kept.txt": {
			Data: []byte("keep me"),
		},
		"ignored.txt": {
			Data: []byte("ignore me"),
		},
	}

	writer := writers.NewMemoryWriter()

	stats, err := renderfs.Copy(source, writer, renderfs.Options{Context: map[string]any{}})
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if stats.Created != 1 {
		t.Fatalf("expected 1 created file, got %d", stats.Created)
	}

	if _, ok := writer.Contents()["ignored.txt"]; ok {
		t.Fatalf("ignored file should not exist")
	}

	if _, ok := writer.Contents()[".renderfs-ignore"]; ok {
		t.Fatalf(".renderfs-ignore should not be copied")
	}

	if _, ok := writer.Contents()["kept.txt"]; !ok {
		t.Fatalf("expected kept.txt to exist")
	}
}

func TestCopyConflictHandling(t *testing.T) {
	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("new"),
		},
	}

	writer := writers.NewMemoryWriter()
	existing, err := writer.CreateFile("file.txt", 0o644)
	if err != nil {
		t.Fatalf("prepare destination file: %v", err)
	}
	if _, err := existing.Write([]byte("original")); err != nil {
		t.Fatalf("write original: %v", err)
	}
	existing.Close()

	stats, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Skip})
	if err != nil {
		t.Fatalf("Copy with skip failed: %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("expected 1 skipped file, got %d", stats.Skipped)
	}
	if got := string(writer.Contents()["file.txt"]); got != "original" {
		t.Fatalf("expected original content preserved, got %q", got)
	}

	if _, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Fail}); err == nil {
		t.Fatalf("expected failure when OnConflict=Fail")
	}
}

func TestCopyIdenticalCounts(t *testing.T) {
	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("same"),
			Mode: 0o644,
		},
	}

	writer := writers.NewMemoryWriter()
	existing, err := writer.CreateFile("file.txt", 0o600)
	if err != nil {
		t.Fatalf("prepare destination file: %v", err)
	}
	if _, err := existing.Write([]byte("same")); err != nil {
		t.Fatalf("write original: %v", err)
	}
	existing.Close()

	stats, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Fail})
	if err != nil {
		t.Fatalf("Copy with identical content failed: %v", err)
	}
	if stats.Identical != 1 {
		t.Fatalf("expected 1 identical file, got %d", stats.Identical)
	}
	if mode, ok := writer.FileMode("file.txt"); !ok || mode != 0o600 {
		t.Fatalf("expected original mode preserved, got %v (ok=%v)", mode, ok)
	}
}

func TestCopyIdenticalSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}

	dest := t.TempDir()
	writer, err := writers.NewOSWriter(dest)
	if err != nil {
		t.Fatalf("NewOSWriter: %v", err)
	}

	targetPath := filepath.Join(dest, "target.txt")
	if err := os.WriteFile(targetPath, []byte("same"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dest, "file.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("same"),
			Mode: 0o644,
		},
	}

	stats, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Fail})
	if err != nil {
		t.Fatalf("Copy with identical symlink target failed: %v", err)
	}
	if stats.Identical != 1 {
		t.Fatalf("expected 1 identical file, got %d", stats.Identical)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "same" {
		t.Fatalf("expected target content unchanged, got %q", got)
	}
}

func TestCopySkipSymlinkDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}

	dest := t.TempDir()
	writer, err := writers.NewOSWriter(dest)
	if err != nil {
		t.Fatalf("NewOSWriter: %v", err)
	}

	targetPath := filepath.Join(dest, "target.txt")
	if err := os.WriteFile(targetPath, []byte("original"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dest, "file.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("new"),
			Mode: 0o644,
		},
	}

	stats, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Skip})
	if err != nil {
		t.Fatalf("Copy with skip symlink failed: %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("expected 1 skipped file, got %d", stats.Skipped)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("expected target content unchanged, got %q", got)
	}
}

func TestCopyDirectoryDestinationErrors(t *testing.T) {
	dest := t.TempDir()
	writer, err := writers.NewOSWriter(dest)
	if err != nil {
		t.Fatalf("NewOSWriter: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dest, "file.txt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("new"),
		},
	}

	if _, err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Skip}); err == nil {
		t.Fatalf("expected error when destination is a directory")
	}
}

func TestCopyMissingVariableRendersEmpty(t *testing.T) {
	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("Hello {{ missing }}!"),
		},
	}

	writer := writers.NewMemoryWriter()

	_, err := renderfs.Copy(source, writer, renderfs.Options{Context: map[string]any{}})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if got := string(writer.Contents()["file.txt"]); got != "Hello !" {
		t.Fatalf("expected missing variable to render as empty, got %q", got)
	}
}

func TestCopyMissingVariableErrorsInStrictMode(t *testing.T) {
	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("Hello {{ missing }}!"),
		},
	}

	writer := writers.NewMemoryWriter()

	_, err := renderfs.Copy(source, writer, renderfs.Options{
		Context:         map[string]any{},
		StrictVariables: true,
	})
	if err == nil {
		t.Fatalf("expected error when StrictVariables is enabled")
	}
}

func TestCopySkipsBinaryByDefault(t *testing.T) {
	source := fstest.MapFS{
		"image.gif": {
			Data: []byte("GIF89a{{ project_name }}"),
		},
	}

	writer := writers.NewMemoryWriter()
	context := map[string]any{
		"project_name": "RenderFS",
	}

	stats, err := renderfs.Copy(source, writer, renderfs.Options{Context: context})
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if stats.Created != 1 {
		t.Fatalf("expected 1 created file, got %d", stats.Created)
	}

	got := writer.Contents()["image.gif"]
	want := []byte("GIF89a{{ project_name }}")
	if !bytes.Equal(got, want) {
		t.Fatalf("expected binary content preserved, got %q", got)
	}
}

func TestCopyTemplatesBinaryWhenEnabled(t *testing.T) {
	source := fstest.MapFS{
		"image.gif": {
			Data: []byte("GIF89a{{ project_name }}"),
		},
	}

	writer := writers.NewMemoryWriter()
	context := map[string]any{
		"project_name": "RenderFS",
	}

	_, err := renderfs.Copy(source, writer, renderfs.Options{
		Context:        context,
		TemplateBinary: true,
	})
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	got := writer.Contents()["image.gif"]
	want := []byte("GIF89aRenderFS")
	if !bytes.Equal(got, want) {
		t.Fatalf("expected binary content templated, got %q", got)
	}
}
