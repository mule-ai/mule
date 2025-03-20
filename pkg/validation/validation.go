package validation

import (
	"fmt"

	"github.com/go-logr/logr"
)

var discard = logr.Discard()

type ValidationInput struct {
	Validations []ValidationFunc
	Logger      logr.Logger
	Path        string
}

func Run(in *ValidationInput) (string, error) {
	// run validation attempts
	if len(in.Validations) < 1 {
		return "", nil
	}
	// run validations
	for _, validation := range in.Validations {
		out, err := validation(in.Path)
		if err != nil {
			in.Logger.Error(err, "validation failed", "output", out)
			return out, fmt.Errorf("validation failed")
		}
	}
	return "", nil
}

func Validations() []string {
	validations := make([]string, 0, len(functions))
	for name := range functions {
		validations = append(validations, name)
	}
	return validations
}
