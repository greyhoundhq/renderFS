package renderfs_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/flosch/pongo2/v6"
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

	context := pongo2.Context{
		"project_name": "RenderFS",
		"params": pongo2.Context{
			"app_name": "demo",
		},
	}

	writer := writers.NewMemoryWriter()

	if err := renderfs.Copy(source, writer, renderfs.Options{Context: context}); err != nil {
		t.Fatalf("Copy failed: %v", err)
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
	context := pongo2.Context{
		"params": pongo2.Context{
			"use_docker": false,
		},
	}

	if err := renderfs.Copy(source, writer, renderfs.Options{Context: context}); err != nil {
		t.Fatalf("Copy failed: %v", err)
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

	if err := renderfs.Copy(source, writer, renderfs.Options{Context: pongo2.Context{}}); err != nil {
		t.Fatalf("Copy failed: %v", err)
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

	if err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Skip}); err != nil {
		t.Fatalf("Copy with skip failed: %v", err)
	}
	if got := string(writer.Contents()["file.txt"]); got != "original" {
		t.Fatalf("expected original content preserved, got %q", got)
	}

	if err := renderfs.Copy(source, writer, renderfs.Options{OnConflict: renderfs.Fail}); err == nil {
		t.Fatalf("expected failure when OnConflict=Fail")
	}
}

func TestCopyFailsOnMissingVariable(t *testing.T) {
	source := fstest.MapFS{
		"file.txt": {
			Data: []byte("{{ missing }}"),
		},
	}

	writer := writers.NewMemoryWriter()

	err := renderfs.Copy(source, writer, renderfs.Options{Context: pongo2.Context{}})
	if err == nil {
		t.Fatalf("expected missing variable error")
	}
}
