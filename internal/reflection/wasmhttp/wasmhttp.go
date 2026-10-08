// Package wasmhttp is the thin host-HTTP bridge the reflection wasm
// modules use. The mule engine registers `http_request_with_headers`
// + `get_last_response_body` + `get_last_response_status` on the
// "env" host module (internal/engine/wasm.go); this package declares
// the imports and wraps them in one Request() call.
//
// It is wasm-only by intent: the //go:wasmimport declarations make it
// useless (and untestable) outside wazero, which is why the reflection
// logic that SHOULD be host-testable lives in the `gate` and
// `reflmem` packages instead.
package wasmhttp

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// The host function declarations (http_request_with_headers & co.)
// live in wasmimports.go (wasip1 build tag) with host stubs in
// wasmimports_stub.go.

// Request performs method fullURL (authKey as Bearer header) with an
// optional JSON body and returns the response body + status. Any host
// failure surfaces as an error — callers turn it into a failed step.
func Request(method, fullURL, body, authKey string) ([]byte, int, error) {
	methodB := []byte(method)
	urlB := []byte(fullURL)
	hdrs := []byte(fmt.Sprintf(`{"Authorization":"Bearer %s","Content-Type":"application/json"}`, authKey))
	var pBody uintptr
	var bodyB []byte
	if body != "" {
		bodyB = []byte(body)
		pBody = uintptr(unsafe.Pointer(&bodyB[0]))
	}
	code := http_request_with_headers(
		uintptr(unsafe.Pointer(&methodB[0])), uintptr(len(methodB)),
		uintptr(unsafe.Pointer(&urlB[0])), uintptr(len(urlB)),
		pBody, uintptr(len(bodyB)),
		uintptr(unsafe.Pointer(&hdrs[0])), uintptr(len(hdrs)))
	if code != 0 {
		return nil, 0, fmt.Errorf("host http_request_with_headers failed: 0x%x", code)
	}
	status := get_last_response_status()
	if status >= 0xFFFFFF00 {
		return nil, 0, fmt.Errorf("no response available: 0x%x", status)
	}
	size := get_last_response_body(0, 0)
	if size >= 0xFFFFFF00 {
		return nil, 0, fmt.Errorf("no response body: 0x%x", size)
	}
	buf := make([]byte, int(size))
	if size > 0 {
		if got := get_last_response_body(uintptr(unsafe.Pointer(&buf[0])), uintptr(size)); got != size {
			return nil, 0, fmt.Errorf("response body read failed: 0x%x", got)
		}
	}
	return buf, int(status), nil
}

// JSON is Request + response decode (body may be empty when into == nil).
func JSON(method, fullURL, body, authKey string, into interface{}) (int, error) {
	raw, status, err := Request(method, fullURL, body, authKey)
	if err != nil {
		return 0, err
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			return status, fmt.Errorf("bad JSON from %s: %v (body: %.200s)", fullURL, err, string(raw))
		}
	}
	return status, nil
}
