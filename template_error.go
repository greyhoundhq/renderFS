package renderfs

import (
	"errors"
	"regexp"
	"strconv"
)

var (
	templateLineRe   = regexp.MustCompile(`line (\d+)`)
	templateFilterRe = regexp.MustCompile(`filter '([^']+)' not found`)
	templateNameRe   = regexp.MustCompile(`(?i)unable to evaluate name "([^"]+)"`)
	templateItemRe   = regexp.MustCompile(`item '([^']+)' not found`)
)

func classifyTemplateError(err error) error {
	if err == nil {
		return nil
	}
	var existing *TemplateError
	if errors.As(err, &existing) {
		return err
	}

	msg := err.Error()
	tErr := &TemplateError{
		Kind: TemplateErrorUnknown,
		Err:  err,
	}

	if match := templateLineRe.FindStringSubmatch(msg); len(match) > 1 {
		if line, convErr := strconv.Atoi(match[1]); convErr == nil {
			tErr.Line = line
		}
	}

	switch {
	case templateFilterRe.MatchString(msg):
		tErr.Kind = TemplateErrorMissingFilter
		tErr.Name = templateFilterRe.FindStringSubmatch(msg)[1]
	case templateNameRe.MatchString(msg):
		tErr.Kind = TemplateErrorMissingVariable
		tErr.Name = templateNameRe.FindStringSubmatch(msg)[1]
	case templateItemRe.MatchString(msg):
		tErr.Kind = TemplateErrorMissingItem
		tErr.Name = templateItemRe.FindStringSubmatch(msg)[1]
	}

	return tErr
}
