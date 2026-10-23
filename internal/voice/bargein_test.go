package voice

import "testing"

func TestBargeInTriggersAfterConsecutiveHotFrames(t *testing.T) {
	b := NewBargeIn(600, 3)

	// Cold frames: no trigger, the hit counter never progresses.
	for i := 0; i < 10; i++ {
		if b.Feed(100) {
			t.Fatalf("cold frame %d triggered barge-in", i)
		}
	}
	// Two hot frames (below the 3-frame hysteresis): no trigger yet.
	if b.Feed(900) {
		t.Fatal("first hot frame triggered")
	}
	if b.Feed(900) {
		t.Fatal("second hot frame triggered")
	}
	// A cold frame between resets the streak.
	if b.Feed(100) {
		t.Fatal("cold frame after hot streak triggered")
	}
	// Three hot frames in a row: trigger, exactly on the third.
	if b.Feed(900) {
		t.Fatal("first of the deciding streak triggered")
	}
	if b.Feed(900) {
		t.Fatal("second of the deciding streak triggered")
	}
	if !b.Feed(900) {
		t.Fatal("third hot frame should trigger barge-in")
	}
	// Single-shot: nothing more until Reset.
	if b.Feed(9000) {
		t.Error("triggered again without Reset")
	}
	b.Reset()
	for i := 0; i < 2; i++ {
		if b.Feed(900) {
			t.Error("post-Reset machine triggered before the 3rd hot frame")
		}
	}
	if !b.Feed(900) {
		t.Error("post-Reset machine should re-trigger on 3 hot frames")
	}
}

func TestBargeInDefaults(t *testing.T) {
	b := NewBargeIn(0, 0)
	if b.Threshold == 0 || b.HitFrames == 0 {
		t.Errorf("defaults not applied: %+v", b)
	}
}

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"On it. Checking the logs now.", []string{"On it.", "Checking the logs now."}},
		{"Done!", []string{"Done!"}},
		{"Is this ok? Yes.", []string{"Is this ok?", "Yes."}},
		{"No punctuation at all", []string{"No punctuation at all"}},
		{"Quote \"this\".", []string{"Quote \"this\"."}},
		{"(parenthetical)", []string{"(parenthetical)"}},
		{"", []string{""}},
	}
	for _, tc := range cases {
		got := splitSentences(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitSentences(%q) = %q, want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitSentences(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
