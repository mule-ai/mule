// Package gate is the deterministic reflection gate (H4.4 plan step 3,
// "contradiction & duplication check + confidence arithmetic"). It is
// pure Go with no HTTP and no wasm imports: the check wasm module is a
// thin host-HTTP + JSON shell around these functions, and the unit
// tests here run on the host against the same logic the workflow ships.
//
// v1 heuristics (documented honestly — see examples/reflection/README.md):
//
//   - duplicate: normalized-string equality (case/whitespace-insensitive)
//     against pending+active beliefs, plus batch-local dedupe. This
//     mirrors forge's own near-duplicate gate (string-primary; the
//     cosine pass lives in forge and is not reachable here because the
//     memory API does not return embeddings).
//   - contradiction: naive opposite-signal detection — negation markers
//     ("never", "don't", "do not", "stop", "avoid", "no longer", ...)
//     present in exactly one of the pair AND content-token overlap
//     (Jaccard over stopword-filtered lowercase tokens) >= 0.4. This is
//     a deliberately conservative v1 heuristic: when it fires, the diff
//     is flagged for human review, never auto-applied; when it misses,
//     forge's cosine gate (0.92) at proposal time is the backstop.
//   - decay: stale active beliefs decay as confidence * exp(-lambda *
//     age_days), lambda default 0.02/day (30 days -> factor 0.5488).
//     Report-only in v1: the proposals endpoint is add-only, so a
//     "weaken" of an existing belief would be rejected as a duplicate.
package gate

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// DefaultLambdaPerDay is the confidence-decay constant (H4.4: lambda
// configurable; 0.02/day means half-life ~34.7 days).
const DefaultLambdaPerDay = 0.02

// OverlapThreshold is the minimum Jaccard token overlap for a
// contradiction flag to fire.
const OverlapThreshold = 0.4

// MinimumDelta is the minimum confidence drop for a decayed-belief
// report entry (absolute confidence points).
const MinimumDelta = 0.05

// Existing is an existing belief the gate compares against.
type Existing struct {
	ID         string
	Content    string
	Confidence float64
	Status     string // active | pending
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Proposed is one belief diff from the reflection agent.
type Proposed struct {
	Op             string
	Belief         string
	Confidence     float64
	Rationale      string
	SourceEpisodes []string
}

// Verdict is the gate's decision for one proposed diff.
type Verdict struct {
	Proposed
	Reason      string
	NeedsReview bool
	Duplicate   bool
}

// Decayed reports a stale belief whose confidence has dropped below
// (old - MinimumDelta).
type Decayed struct {
	ID        string
	Content   string
	OldConf   float64
	NewConf   float64
	AgeDays   float64
	AgeAnchor time.Time // max(created_at, updated_at) — last changed
}

// Report is the gate's full output.
type Report struct {
	Safe        []Verdict
	NeedsReview []Verdict
	Decayed     []Decayed
}

// Normalize lowercases and collapses all runs of whitespace to one
// space (the same family of normalization forge's string dup gate
// uses).
func Normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// decayFactor returns exp(-lambda * ageDays).
func decayFactor(ageDays, lambda float64) float64 {
	return math.Exp(-lambda * ageDays)
}

// stopwords are dropped before overlap comparison so negation words and
// function words don't dominate the token set.
var stopWords = map[string]bool{
	"a": true, "about": true, "after": true, "all": true, "also": true,
	"always": true, "and": true, "an": true, "any": true, "as": true,
	"at": true, "because": true, "be": true, "been": true, "before": true,
	"being": true, "but": true, "by": true, "can": true, "cannot": true,
	"could": true, "did": true, "does": true, "doing": true, "down": true,
	"during": true, "each": true, "either": true, "else": true, "for": true,
	"from": true, "get": true, "had": true, "has": true, "have": true,
	"having": true, "he": true, "her": true, "here": true, "him": true,
	"his": true, "i": true, "if": true, "in": true, "into": true, "is": true,
	"it": true, "its": true, "just": true, "like": true, "may": true,
	"might": true, "more": true, "most": true, "must": true, "my": true,
	"neither": true, "no": true, "nor": true, "not": true, "now": true,
	"of": true, "off": true, "on": true, "once": true, "only": true, "or": true,
	"other": true, "our": true, "out": true, "over": true, "own": true,
	"prefer": true, "preferable": true, "rather": true, "should": true,
	"some": true, "such": true, "than": true, "that": true, "the": true,
	"their": true, "them": true, "then": true, "there": true, "these": true,
	"they": true, "this": true, "to": true, "too": true, "under": true,
	"until": true, "up": true, "user": true, "users": true, "very": true,
	"was": true, "we": true, "were": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "who": true, "why": true,
	"will": true, "with": true, "would": true, "yes": true, "you": true,
	"your": true, "theirs": true, "ours": true,
}

// negationMarkers are word-boundary markers of a negative assertion.
// "never" is the canonical example: "the user NEVER wants verbose
// replies" vs "the user wants verbose replies".
var negationMarkers = []string{
	"never", "don't", "dont", "do not", "does not", "did not", "stop",
	"stops", "stopped", "stopping", "avoid", "avoids", "avoiding",
	"no longer", "no more", "refuse", "refuses", "refused", "rejection",
	"without", "prohibit", "forbidden", "ban",
}

// tokens lowercases, strips punctuation, drops stopwords, and
// dedupes.
func tokens(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,;:!?\"'()[]{}<>")
		w = strings.TrimFunc(w, func(r rune) bool {
			return unicode.IsPunct(r) || unicode.IsSymbol(r)
		})
		if w == "" || stopWords[w] {
			continue
		}
		set[w] = true
	}
	return set
}

// overlap is the Jaccard similarity of the two token sets.
func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	uni := len(a) + len(b) - inter
	return float64(inter) / float64(uni)
}

// hasNegation reports whether s contains a negation marker on a word
// boundary.
func hasNegation(s string) bool {
	s = " " + strings.ToLower(s) + " "
	for _, m := range negationMarkers {
		if strings.Contains(s, " "+m+" ") {
			return true
		}
	}
	return false
}

// Contradicts reports whether two beliefs assert opposite things, via
// the v1 opposite-signal heuristic: negation present in exactly one of
// the pair AND token overlap >= OverlapThreshold. Both-negative pairs
// ("never X" vs "avoid X") are NOT flagged — they agree in polarity.
// A clean negation-free pair is never flagged here; semantic opposition
// without markers is forge's cosine/dup-gate territory at submit time.
func Contradicts(a, b string) bool {
	na, nb := hasNegation(a), hasNegation(b)
	if na == nb {
		return false
	}
	return overlap(tokens(a), tokens(b)) >= OverlapThreshold
}

// IsDuplicate reports normalized-string equality (case/whitespace
// insensitive).
func IsDuplicate(a, b string) bool {
	return Normalize(a) == Normalize(b)
}

// ClampConfidence clamps to [0,1]; NaN/missing -> 0.5 (forge's default).
func ClampConfidence(c float64) float64 {
	if math.IsNaN(c) {
		return 0.5
	}
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// Gate runs the deterministic gate over the proposed diffs:
//
//  1. ops the forge proposals endpoint cannot express in v1
//     (strengthen/weaken/forget/supersede — anything that is not "add")
//     -> needs_review (they still get submitted with a review prefix so
//     a human can act on the card; the endpoint stores them as pending
//     beliefs, and forget/supersede is then applied by hand via the
//     keep/forget doors).
//  2. exact duplicates of existing (pending/active) beliefs ->
//     needs_review + Duplicate (NOT resubmitted — forge's dup gate
//     rejects them; the point of the card is the human seeing WHY).
//  3. contradictions (v1 heuristic) against existing beliefs ->
//     needs_review, never auto-applied.
//  4. everything else -> safe.
//
// Batch-local duplicates: the second copy of an identical proposal in
// one batch is needs_review (Duplicate) — the agent may emit the same
// diff for two overlapping episodes.
//
// decayLambda <= 0 disables the decay report.
func Gate(proposed []Proposed, existing []Existing, now time.Time, decayLambda float64) *Report {
	rep := &Report{}
	existingNorms := make(map[string]Existing, len(existing))
	for _, e := range existing {
		n := Normalize(e.Content)
		if _, seen := existingNorms[n]; !seen {
			existingNorms[n] = e
		}
	}
	seenInBatch := map[string]int{}

	for _, p := range proposed {
		c := p.Confidence
		if c == 0 { // JSON absence decodes to 0: forge's default is 0.5
			c = 0.5
		}
		v := Verdict{Proposed: Proposed{
			Op: p.Op, Belief: p.Belief,
			Confidence: ClampConfidence(c),
			Rationale:  p.Rationale, SourceEpisodes: p.SourceEpisodes,
		}}
		n := Normalize(p.Belief)
		if n == "" {
			v.Reason = "empty belief content"
			v.NeedsReview = true
			rep.NeedsReview = append(rep.NeedsReview, v)
			continue
		}
		seenInBatch[n]++
		switch strings.ToLower(strings.TrimSpace(p.Op)) {
		case "", "add":
			if e, dup := existingNorms[n]; dup {
				v.Reason = "duplicate of existing belief " + e.ID
				v.NeedsReview = true
				v.Duplicate = true
				rep.NeedsReview = append(rep.NeedsReview, v)
				continue
			}
			if seenInBatch[n] > 1 {
				v.Reason = "duplicate within this reflection batch"
				v.NeedsReview = true
				v.Duplicate = true
				rep.NeedsReview = append(rep.NeedsReview, v)
				continue
			}
			for _, e := range existing {
				if Contradicts(p.Belief, e.Content) {
					v.Reason = "possible contradiction with existing belief " + e.ID + ": " + e.Content
					v.NeedsReview = true
					rep.NeedsReview = append(rep.NeedsReview, v)
					break
				}
			}
			if !v.NeedsReview {
				rep.Safe = append(rep.Safe, v)
			}
		default:
			v.Reason = "op " + p.Op + " needs human review (v1 proposals endpoint is add-only)"
			v.NeedsReview = true
			rep.NeedsReview = append(rep.NeedsReview, v)
		}
	}

	// Decay pass over the ACTIVE beliefs (pending beliefs have not been
	// confirmed and are not yet "memory" that decays).
	if decayLambda > 0 {
		for _, e := range existing {
			if strings.ToLower(e.Status) != "active" {
				continue
			}
			anchor := e.CreatedAt
			if e.UpdatedAt.After(anchor) {
				anchor = e.UpdatedAt
			}
			ageDays := now.Sub(anchor).Hours() / 24
			if ageDays < 0 {
				ageDays = 0
			}
			newConf := e.Confidence * decayFactor(ageDays, decayLambda)
			if e.Confidence-newConf >= MinimumDelta {
				rep.Decayed = append(rep.Decayed, Decayed{
					ID: e.ID, Content: e.Content,
					OldConf: e.Confidence, NewConf: newConf,
					AgeDays: ageDays, AgeAnchor: anchor,
				})
			}
		}
	}
	return rep
}
