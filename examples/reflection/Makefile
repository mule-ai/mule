.PHONY: build clean test

# Herd H4.4 reflection workflow wasm modules.
# Same toolchain as examples/wasm/*: the standard Go compiler for
# GOOS=wasip1 (no TinyGo needed; the modules use no CGO).

DIST := dist
MODULES := pull check submit stub

build:
	@mkdir -p $(DIST)
	@for m in $(MODULES); do \
		echo "building $(DIST)/$$m.wasm ..."; \
		GOOS=wasip1 GOARCH=wasm go build -o $(DIST)/$$m.wasm $$m/main.go || exit 1; \
	done
	@ls -la $(DIST)

# Host-side tests for the shared pure logic (gate + reflmem). The wasm
# binaries themselves are exercised end to end by
# internal/engine/reflection_test.go (builds them if dist/ is empty).
test:
	go test -count=1 ./internal/reflection/...

clean:
	rm -rf $(DIST)
