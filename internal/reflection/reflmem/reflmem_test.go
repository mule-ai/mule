package reflmem

import (
	"testing"
)

func TestParseProposals_BareArray(t *testing.T) {
	text := `[{"op":"add","belief":"the user wants brief replies","confidence":0.8,"rationale":"asked twice","source_episodes":["a","b"]}]`
	out, err := ParseProposals(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Op != "add" || out[0].Belief != "the user wants brief replies" || out[0].Confidence != 0.8 || len(out[0].SourceEpisodes) != 2 {
		t.Fatalf("got %+v", out)
	}
}

func TestParseProposals_CodeFence(t *testing.T) {
	text := "Here are the diffs I found:\n```json\n[{\"op\":\"forget\",\"belief\":\"old deploy time\",\"confidence\":0.9}]\n```\nHappy to refine."
	out, err := ParseProposals(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Op != "forget" {
		t.Fatalf("got %+v", out)
	}
}

func TestParseProposals_BracketsInsideStrings(t *testing.T) {
	// A `]` inside a JSON string must not end the array early.
	text := `nonsense [not json] preamble [{"op":"add","belief":"use [] brackets in bash arrays","rationale":"x"}] tail`
	out, err := ParseProposals(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Belief != "use [] brackets in bash arrays" {
		t.Fatalf("got %+v", out)
	}
}

func TestParseProposals_EmptyArrayIsLegit(t *testing.T) {
	out, err := ParseProposals("nothing worth remembering this week: []")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %+v", out)
	}
}

func TestParseProposals_NoArray(t *testing.T) {
	out, err := ParseProposals("no learnings to report")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %+v", out)
	}
}

func TestParseProposals_BadJSON(t *testing.T) {
	if _, err := ParseProposals("[{op add]"); err == nil {
		t.Fatal("expected error on unclosed/invalid JSON")
	}
	if _, err := ParseProposals(`[{"op":`); err == nil {
		t.Fatal("expected error on truncated JSON")
	}
}

func TestUnwrapStepInput(t *testing.T) {
	// Envelope with an object payload (wasm step -> next wasm step):
	// payload keys merge in WITHOUT losing the sibling envelope keys
	// (the forge creds ride along next to "prompt").
	in := map[string]interface{}{
		"prompt":        map[string]interface{}{"safe": []interface{}{}},
		"forge_url":     "http://u",
		"forge_api_key": "k",
	}
	m, err := UnwrapStepInput(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["safe"]; !ok {
		t.Fatalf("want merged payload key, got %v", m)
	}
	if m["forge_url"] != "http://u" {
		t.Fatalf("envelope sibling keys must survive the merge: %v", m)
	}
	// Payload keys win on conflict.
	in2 := map[string]interface{}{"prompt": map[string]interface{}{"k": "payload"}, "k": "envelope"}
	m2, _ := UnwrapStepInput(in2)
	if m2["k"] != "payload" {
		t.Fatalf("payload should win: %v", m2)
	}
	// Agent text envelope: prompt is a string -> stays as-is.
	in3 := map[string]interface{}{"prompt": "prose from the model"}
	m3, _ := UnwrapStepInput(in3)
	if s, _ := m3["prompt"].(string); s != "prose from the model" {
		t.Fatalf("want agent text preserved, got %v", m3)
	}
	// Bare payload: unchanged.
	in4 := map[string]interface{}{"forge_url": "x"}
	m4, _ := UnwrapStepInput(in4)
	if m4["forge_url"] != "x" {
		t.Fatalf("got %v", m4)
	}
}

func TestCredsFromInput(t *testing.T) {
	ok := false
	_, _, _, ok = CredsFromInput(map[string]interface{}{"forge_url": "u", "agent_id": "a"})
	if ok {
		t.Fatal("missing api key must not be ok")
	}
	_, _, _, ok = CredsFromInput(map[string]interface{}{"forge_url": "u", "forge_api_key": "k", "agent_id": "a"})
	if !ok {
		t.Fatal("full creds must be ok")
	}
}
