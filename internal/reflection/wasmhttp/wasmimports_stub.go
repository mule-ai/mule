//go:build !wasip1

package wasmhttp

// Host-side stand-ins so the package type-checks/links on the host
// toolchain (go build ./..., go vet ./...). Request() panics on the
// host: HTTP only exists inside the mule engine's wazero runtime,
// which is where these modules actually run. The logic worth testing
// lives in ../gate and ../reflmem, which have no build tags.
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr {
	panic("wasmhttp: host functions are only available inside the mule wasm runtime")
}

func get_last_response_body(bufferPtr, bufferSize uintptr) uint32 {
	panic("wasmhttp: host functions are only available inside the mule wasm runtime")
}

func get_last_response_status() uint32 {
	panic("wasmhttp: host functions are only available inside the mule wasm runtime")
}
