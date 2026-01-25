package renderfs

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
)

var templateCache sync.Map // map[string]*exec.Template

// RenderBytes renders template bytes using the provided context.
// When templateBinary is false, binary content is returned unchanged.
func RenderBytes(raw []byte, ctx map[string]any, templateBinary bool, strict bool) ([]byte, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	if !templateBinary && isBinary(raw) {
		return raw, nil
	}
	rendered, err := renderTemplateString(string(raw), ctx, strict)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

func renderTemplateString(tpl string, ctx map[string]any, strict bool) (string, error) {
	compiled, err := getOrCompileTemplate(tpl, strict)
	if err != nil {
		return "", err
	}

	out, err := compiled.ExecuteToString(exec.NewContext(ctx))
	if err != nil {
		return "", classifyTemplateError(err)
	}
	return out, nil
}

func getOrCompileTemplate(tpl string, strict bool) (*exec.Template, error) {
	cacheKey := fmt.Sprintf("%t:%s", strict, tpl)
	if cached, ok := templateCache.Load(cacheKey); ok {
		return cached.(*exec.Template), nil
	}

	cfg := config.New()
	cfg.StrictUndefined = strict

	sum := sha256.Sum256([]byte(tpl))
	rootID := fmt.Sprintf("root-%x", sum[:])

	loader, err := loaders.NewFileSystemLoader("")
	if err != nil {
		return nil, err
	}
	shiftedLoader, err := loaders.NewShiftedLoader(rootID, bytes.NewReader([]byte(tpl)), loader)
	if err != nil {
		return nil, err
	}

	compiled, err := exec.NewTemplate(rootID, cfg, shiftedLoader, gonja.DefaultEnvironment)
	if err != nil {
		return nil, err
	}

	templateCache.Store(cacheKey, compiled)
	return compiled, nil
}
