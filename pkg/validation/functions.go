package validation

import "os/exec"

type ValidationFunc func(string) (string, error)

var functions = map[string]ValidationFunc{
	"getDeps":      getDeps,
	"goFmt":        goFmt,
	"goModTidy":    goModTidy,
	"golangciLint": golangciLint,
	"goTest":       goTest,
}

func Get(name string) (ValidationFunc, bool) {
	if fn, ok := functions[name]; ok {
		return fn, true
	}
	return nil, false
}

func goFmt(path string) (string, error) {
	cmd := exec.Command("go", "fmt", "./...")
	cmd.Dir = path
	_, err := cmd.CombinedOutput()
	if err != nil {
		discard.Info("go fmt updated files, ignoring error")
	}
	return "", nil
}

func goModTidy(path string) (string, error) {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = path
	_, err := cmd.CombinedOutput()
	if err != nil {
		discard.Info("go mod tidy failed, ignoring error")
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
		discard.Info("download golangci-lint failed")
	}
	return "", nil
}
