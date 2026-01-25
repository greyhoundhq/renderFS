package renderfs

import "fmt"

type RenderErrorKind string

const (
	RenderErrorPath     RenderErrorKind = "path"
	RenderErrorFile     RenderErrorKind = "file"
	RenderErrorConflict RenderErrorKind = "conflict"
)

type RenderError struct {
	Kind RenderErrorKind
	Path string
	Err  error
}

func (e *RenderError) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case RenderErrorPath:
		if e.Err != nil {
			return fmt.Sprintf("renderfs: render path %s: %v", e.Path, e.Err)
		}
		return fmt.Sprintf("renderfs: render path %s", e.Path)
	case RenderErrorFile:
		if e.Err != nil {
			return fmt.Sprintf("renderfs: render file %s: %v", e.Path, e.Err)
		}
		return fmt.Sprintf("renderfs: render file %s", e.Path)
	case RenderErrorConflict:
		return fmt.Sprintf("renderfs: destination file %s exists and differs", e.Path)
	default:
		if e.Err != nil {
			return fmt.Sprintf("renderfs: %s: %v", e.Path, e.Err)
		}
		return fmt.Sprintf("renderfs: %s", e.Path)
	}
}

func (e *RenderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type TemplateErrorKind string

const (
	TemplateErrorUnknown         TemplateErrorKind = "unknown"
	TemplateErrorMissingVariable TemplateErrorKind = "missing_variable"
	TemplateErrorMissingFilter   TemplateErrorKind = "missing_filter"
	TemplateErrorMissingItem     TemplateErrorKind = "missing_item"
)

type TemplateError struct {
	Kind TemplateErrorKind
	Name string
	Line int
	Err  error
}

func (e *TemplateError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *TemplateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
