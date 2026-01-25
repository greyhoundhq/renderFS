package renderfs

import (
	"fmt"
	"path"
	"strings"

	"github.com/nikolalohinski/gonja/v2/exec"
)

// RenderPath renders a template path and validates that it stays within destination.
// It returns the cleaned path, whether it should be skipped, and any error encountered.
func RenderPath(rel string, isDir bool, ctx map[string]any, strict bool) (string, bool, error) {
	return RenderPathWithEnv(rel, isDir, ctx, strict, nil)
}

// RenderPathWithEnv renders a template path and validates that it stays within destination.
// It returns the cleaned path, whether it should be skipped, and any error encountered.
func RenderPathWithEnv(rel string, isDir bool, ctx map[string]any, strict bool, env *exec.Environment) (string, bool, error) {
	rendered, err := renderTemplateString(rel, ctx, strict, env)
	if err != nil {
		return "", false, err
	}

	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", true, nil
	}

	rendered = strings.ReplaceAll(rendered, "\\", "/")
	clean := path.Clean(rendered)
	if clean == "." {
		return "", true, nil
	}

	if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false, fmt.Errorf("renderfs: rendered path %q escapes destination", rendered)
	}
	if isWindowsAbs(clean) {
		return "", false, fmt.Errorf("renderfs: rendered path %q escapes destination", rendered)
	}

	if !isDir {
		base := path.Base(clean)
		stripped := stripTemplateSuffix(base)
		if stripped == "" || stripped == "." {
			return "", true, nil
		}
		clean = path.Join(path.Dir(clean), stripped)
	}

	return clean, false, nil
}

func isWindowsAbs(value string) bool {
	if strings.HasPrefix(value, "//") {
		return true
	}
	if len(value) >= 2 && value[1] == ':' {
		return true
	}
	return false
}
