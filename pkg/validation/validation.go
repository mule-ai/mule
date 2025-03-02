package validation

import (
	"fmt"

	"github.com/go-logr/logr"
)

var discard = logr.Discard()

type ValidationInput struct {
	Attempts    int
	Validations []ValidationFunc
	Send        chan<- string
	Done        <-chan bool
	Logger      logr.Logger
	Path        string
}

func Run(in *ValidationInput) error {
	// run validation attempts
	validated := false
	for i := 0; i < in.Attempts; i++ {
		// run validations
		for _, validation := range in.Validations {
			out, err := validation(in.Path)
			if err != nil {
				validated = false
				errString := fmt.Sprintf("validation %d out of %d failed", i, in.Attempts)
				in.Logger.Error(err, errString, "output", out)
				in.Send <- out
				<-in.Done
				break
			}
			validated = true
		}
		if validated {
			return nil
		}
	}
	if !validated {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func Validations() []string {
	validations := make([]string, 0, len(functions))
	for name := range functions {
		validations = append(validations, name)
	}
	return validations
}
