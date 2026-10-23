//go:build ignore

// stub — a stand-in for the reflection agent step (step 2).
//
// The real workflow's step 2 is a MULE AGENT STEP (a cheap model with
// the reflection system prompt in seed.sh). The agent step cannot be
// faked in-process — it spawns `pi --mode rpc` — so:
//
//   - the engine-level e2e test (internal/engine/reflection_test.go)
//     swaps this module in for step 2: a full 4-step job (pull ->
//     stub -> check -> submit) runs end to end against a fake forge;
//     the real-model leg is the manual acceptance.
//   - a human can run the loop with no LLM at all to sanity-check the
//     pull/check/submit wiring.
//
// The output deliberately mirrors an agent's final MESSAGE: prose
// around a ```json fenced array (exercises reflmem.ParseProposals'
// fence path), so check receives {"prompt": <this text>} exactly like
// the agent leg.
//
// Config key: stub_text (string). Defaults to the built-in demo text,
// whose first proposal is crafted to duplicate the fake forge's
// seeded belief so the e2e test can assert the gate held it.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o stub.wasm stub/main.go

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const defaultStubText = "Reflection complete. Proposed belief diffs:\n" +
	"```json\n" +
	"[\n" +
	"  {\"op\": \"add\", \"belief\": \"the user wants brief replies\", \"confidence\": 0.9,\n" +
	"   \"rationale\": \"echoes the existing active belief (duplicate on purpose in this demo)\",\n" +
	"   \"source_episodes\": [\"00000000-0000-0000-0000-000000000001\"]},\n" +
	"  {\"op\": \"add\", \"belief\": \"the user deploys only after a staging pass\", \"confidence\": 0.8,\n" +
	"   \"rationale\": \"two episodes show staging-first deploys\",\n" +
	"   \"source_episodes\": [\"00000000-0000-0000-0000-0000-000000000001\",\n" +
	"                        \"00000000-0000-0000-0000-000000000002\"]},\n" +
	"  {\"op\": \"forget\", \"belief\": \"the user wants long-form replies\", \"confidence\": 0.4,\n" +
	"   \"rationale\": \"superseded by the brief-replies pattern\"}\n" +
	"]\n" +
	"```\n" +
	"End of reflection."

func main() {
	var in map[string]interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		in = map[string]interface{}{}
	}
	text := defaultStubText
	if s, ok := in["stub_text"].(string); ok && s != "" {
		text = s
	}
	// Raw (non-JSON) stdout on purpose: the engine passes it through
	// as the step's "prompt" string, exactly like an agent step would.
	fmt.Print(text)
}
