package gate

import (
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	if got := Normalize("The  USER wants\tbrief\nreplies"); got != "the user wants brief replies" {
		t.Fatalf("Normalize = %q", got)
	}
	if !IsDuplicate("The user wants brief replies", "the   USER  WANTS brief replies") {
		t.Fatal("expected duplicate under case/whitespace normalization")
	}
	if IsDuplicate("the user wants brief replies", "the user wants short answers") {
		t.Fatal("distinct content flagged as duplicate")
	}
}

func TestContradicts_NegationFlip(t *testing.T) {
	a := "the user never wants verbose replies"
	b := "the user wants verbose replies"
	if !Contradicts(a, b) {
		t.Fatalf("expected contradiction between %q and %q", a, b)
	}
	if !Contradicts(b, a) {
		t.Fatalf("contradiction is asymmetric; got no flag in reverse")
	}
	// "stop" marker.
	if !Contradicts("the agent should stop sending the weekly report", "the agent sends the weekly report on friday") {
		t.Fatal("stop-marker flip not flagged")
	}
}

func TestContradicts_NoFalsePositives(t *testing.T) {
	// Same polarity, high overlap: both negative — they agree.
	if Contradicts("the user never wants verbose replies", "the user avoids verbose replies") {
		t.Fatal("both-negative pair flagged as contradiction")
	}
	// No negation at all: agreement, not contradiction.
	if Contradicts("the user wants verbose replies", "the user wants very verbose replies") {
		t.Fatal("affirmative pair flagged as contradiction")
	}
	// Opposite polarity but disjoint content: overlap below threshold.
	if Contradicts("the user never wants verbose replies", "the user always wears a helmet while cycling") {
		t.Fatal("disjoint content flagged as contradiction")
	}
}

func TestDecayMath(t *testing.T) {
	// lambda = 0.02/day, 30 days: factor exp(-0.6) = 0.54881...
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	existing := []Existing{{
		ID: "b1", Content: "the user wants brief replies",
		Confidence: 1.0, Status: "active",
		CreatedAt: now.Add(-30 * 24 * time.Hour),
	}}
	rep := Gate(nil, existing, now, DefaultLambdaPerDay)
	if len(rep.Decayed) != 1 {
		t.Fatalf("Decayed = %+v, want one entry", rep.Decayed)
	}
	d := rep.Decayed[0]
	want := 1.0 * decayFactor(30, DefaultLambdaPerDay) // exp(-0.6) ≈ 0.54881164
	if d.NewConf < 0.5488 || d.NewConf > 0.5489 {
		t.Fatalf("NewConf = %v, want ≈ %v (in [0.5488, 0.5489])", d.NewConf, want)
	}
	if d.AgeDays < 29.9 || d.AgeDays > 30.1 {
		t.Fatalf("AgeDays = %v, want ~30", d.AgeDays)
	}
	// A 1-day-old belief must not be reported (drop below MinimumDelta).
	existing[0].CreatedAt = now.Add(-24 * time.Hour)
	if rep := Gate(nil, existing, now, DefaultLambdaPerDay); len(rep.Decayed) != 0 {
		t.Fatalf("recent belief decayed: %+v", rep.Decayed)
	}
	// Lambda 0 disables the decay pass.
	if rep := Gate(nil, existing, now, 0); len(rep.Decayed) != 0 {
		t.Fatalf("decay pass ran with lambda 0: %+v", rep.Decayed)
	}
	// Pending beliefs do not decay.
	existing[0].Status = "pending"
	if rep := Gate(nil, existing, now, DefaultLambdaPerDay); len(rep.Decayed) != 0 {
		t.Fatalf("pending belief decayed: %+v", rep.Decayed)
	}
}

func TestGate_SafeVsNeedsReview(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	existing := []Existing{
		{ID: "b1", Content: "the user wants brief replies", Confidence: 0.9, Status: "active", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "b2", Content: "deploy on friday afternoons", Confidence: 0.6, Status: "pending", CreatedAt: now.Add(-2 * time.Hour)},
	}
	proposed := []Proposed{
		{Op: "add", Belief: "the user always signs emails with a PGP block", Confidence: 0.8},
		// Case/whitespace duplicate of b1 -> needs_review + duplicate.
		{Op: "add", Belief: "  The User wants brief replies ", Confidence: 0.9},
		// Contradiction of b1 (negation flip) -> needs_review, NOT duplicate.
		{Op: "add", Belief: "the user never wants brief replies", Confidence: 0.7},
		// Duplicate of a PENDING belief also gates.
		{Op: "add", Belief: "Deploy on Friday afternoons", Confidence: 0.7},
		// forget -> human review (endpoint is add-only).
		{Op: "forget", Belief: "the user wants brief replies", Confidence: 0.2},
		// supersede -> human review.
		{Op: "supersede", Belief: "new belief replaces the old one", Confidence: 0.6},
		// Batch-local duplicate: second copy of a safe add.
		{Op: "add", Belief: "the user prefers tabs over spaces", Confidence: 0.5},
		{Op: "add", Belief: "the user prefers tabs over spaces", Confidence: 0.55},
		// Empty content.
		{Op: "add", Belief: "   ", Confidence: 0.5},
	}
	rep := Gate(proposed, existing, now, DefaultLambdaPerDay)

	if len(rep.Safe) != 2 {
		t.Fatalf("Safe = %+v, want exactly [pgp-block, tabs-first-copy]", rep.Safe)
	}
	got := map[string]bool{}
	for _, v := range rep.Safe {
		got[v.Belief] = true
	}
	if !got["the user always signs emails with a PGP block"] {
		t.Fatal("safe set missing the clean add")
	}
	if !got["the user prefers tabs over spaces"] {
		t.Fatal("safe set missing the first batch copy")
	}

	wantReview := 7
	if len(rep.NeedsReview) != wantReview {
		t.Fatalf("NeedsReview = %+v, want %d entries", rep.NeedsReview, wantReview)
	}
	for _, v := range rep.NeedsReview {
		switch {
		case v.Duplicate && v.Reason == "duplicate of existing belief b1":
		case v.Duplicate && v.Reason == "duplicate of existing belief b2":
		case !v.Duplicate && v.Reason != "" && strings.Contains(v.Reason, "contradiction"):
		case v.Reason == "op forget needs human review (v1 proposals endpoint is add-only)":
		case v.Reason == "op supersede needs human review (v1 proposals endpoint is add-only)":
		case v.Duplicate && v.Reason == "duplicate within this reflection batch":
		case v.Reason == "empty belief content":
		default:
			t.Fatalf("unrecognized needs_review verdict: %+v", v)
		}
	}
}

func TestGate_ConfidenceClampAndDefault(t *testing.T) {
	now := time.Now()
	rep := Gate([]Proposed{
		{Op: "add", Belief: "a", Confidence: 1.7},
		{Op: "add", Belief: "b", Confidence: -0.4},
		{Op: "add", Belief: "c", Confidence: 0}, // missing -> forge default 0.5
	}, nil, now, 0)
	if len(rep.Safe) != 3 {
		t.Fatalf("Safe = %+v", rep.Safe)
	}
	if rep.Safe[0].Confidence != 1 || rep.Safe[1].Confidence != 0 || rep.Safe[2].Confidence != 0.5 {
		t.Fatalf("clamps: %v %v %v", rep.Safe[0].Confidence, rep.Safe[1].Confidence, rep.Safe[2].Confidence)
	}
}
