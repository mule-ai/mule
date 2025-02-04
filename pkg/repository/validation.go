package repository

import (
	"fmt"
	"log"
	"os/exec"
)

type ValidationInput struct {
	attempts    int
	validations []func(string) (string, error)
	send        chan<- string
	done        <-chan bool
}

func (r *Repository) validateOutput(in *ValidationInput) error {
	// run validation attempts
	validated := false
	for i := 0; i < in.attempts; i++ {
		// run validations
		for _, validation := range in.validations {
			out, err := validation(r.Path)
			if err != nil {
				validated = false
				log.Printf("validation failed: %s, %s", err, out)
				in.send <- out
				<-in.done
				break
			}
			validated = true
		}
		if validated {
			break
		}
	}
	if !validated {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func goFmt(path string) (string, error) {
	cmd := exec.Command("go", "fmt", "./...")
	cmd.Dir = path
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("go fmt updated files, ignoring error")
	}
	return "", nil
}

func goModTidy(path string) (string, error) {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = path
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("go mod tidy failed, ignoring error")
	}
	return "", nil
}

func golangciLint(path string) (string, error) {
	cmd := exec.Command("./bin/golangci-lint", "run")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	// convert byte array to string
	return string(out), err
}

func goTest(path string) (string, error) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func getDeps(path string) (string, error) {
	cmd := exec.Command("make", "download-golangci-lint")
	cmd.Dir = path
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("download golangci-lint failed")
	}
	return "", nil
}
