.PHONY: build build-projects test clean

WASM_FILE=github-issues.wasm
WASM_PROJECTS_FILE=github-issues-projects.wasm

build: build-basic build-projects

build-basic:
	GOOS=wasip1 GOARCH=wasm go build -o $(WASM_FILE) github_issues.go

build-projects:
	GOOS=wasip1 GOARCH=wasm go build -o $(WASM_PROJECTS_FILE) github_issues_projects.go

test: build
	@echo "Testing requires valid GitHub credentials and repository"
	@echo "See README for usage instructions"

clean:
	rm -f $(WASM_FILE) $(WASM_PROJECTS_FILE)