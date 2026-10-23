package validation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ------------------------------------------------------------------
// Policy rules (Herd H3.5, migration 0015)
//
// A policy rule is one deterministic decision a policy makes about a
// tool call: "when <tool> is called and <match> applies, the verdict is
// <disposition>." Rules are owned by their policy (versioned,
// owner-scoped); the evaluator (internal/policy) picks the winning
// rule by scope specificity + restrictiveness.
//
// Sensitivity tiers are the dots model (research §2.2): `sensitive`
// data → named-recipient rules; destructive/irreversible or money →
// always `ask`; credentials/money transfer → `deny` with a
// "hand-back-to-human" reason.
// ------------------------------------------------------------------

// RuleAction is the tool-call matcher for a rule.
//
//	Tool:  the tool name the rule applies to: "bash" | "write" | "edit"
//	       | "web" | "email" | "*" (any tool).
//	Match: optional matcher over the tool INPUT; semantics in
//	       MatchRule below.
type RuleAction struct {
	Tool  string `json:"tool"`
	Match string `json:"match,omitempty"`
}

// Rule is one entry of a policy's `rules` array.
//
//	Reviewer/ReviewMode: v1 STUB. When the winning rule has
//	Reviewer=true and ReviewMode=="agent", the evaluator marks the
//	verdict review="pending" and returns the DETERMINISTIC verdict
//	immediately (the "deterministic floor"). The review-agent upgrade
//	path (a cheap mule agent step that can only LOOSEN the floor to a
//	documented subset) is a documented follow-up; forge never blocks
//	on a review agent in v1.
type Rule struct {
	ID          string     `json:"id"`
	Description string     `json:"description,omitempty"`
	Action      RuleAction `json:"action"`
	Sensitivity string     `json:"sensitivity"`
	Disposition string     `json:"disposition"`
	Reason      string     `json:"reason,omitempty"`
	Reviewer    bool       `json:"reviewer,omitempty"`
	ReviewMode  string     `json:"review_mode,omitempty"`
}

// Valid dispositions / sensitivities / tools / scopes.
const (
	DispositionAllow = "allow"
	DispositionDeny  = "deny"
	DispositionAsk   = "ask"

	SensitivityLow       = "low"
	SensitivityNormal    = "normal"
	SensitivitySensitive = "sensitive"

	ScopeGlobal = "global"
	ScopeAgent  = "agent"
	ScopeOrg    = "org"
)

// maxGlobLen bounds the match-glob length (parse gate; see ValidateRules).
const maxGlobLen = 512

func validDispositions() map[string]bool {
	return map[string]bool{"allow": true, "deny": true, "ask": true}
}

func validSensitivities() map[string]bool {
	return map[string]bool{"low": true, "normal": true, "sensitive": true}
}

func validTools() map[string]bool {
	return map[string]bool{
		"bash": true, "write": true, "edit": true, "web": true,
		"email": true, "*": true,
	}
}

// ValidatePolicyScope checks the scope value and the agent-scope
// invariant (agent scope requires an agent_id).
func ValidatePolicyScope(scope, agentID string) error {
	switch scope {
	case ScopeGlobal, ScopeOrg:
		return nil
	case ScopeAgent:
		if strings.TrimSpace(agentID) == "" {
			return fmt.Errorf("scope 'agent' requires agent_id")
		}
		return nil
	default:
		return fmt.Errorf("scope must be one of 'global','agent','org' (got %q)", scope)
	}
}

// ValidateRules validates a full policy rules array. It enforces:
//   - non-empty rules array,
//   - unique non-empty rule ids,
//   - disposition ∈ {allow, deny, ask},
//   - sensitivity ∈ {low, normal, sensitive},
//   - action.tool ∈ {bash, write, edit, web, email, *},
//   - action.match: a glob that PARSES (compiles) — see globToRegex;
//     longer than maxGlobLen is rejected. (An empty match string is
//     accepted and means "match every call of this tool".)
func ValidateRules(rules []Rule) error {
	if len(rules) == 0 {
		return fmt.Errorf("rules must be a non-empty array")
	}
	seen := map[string]bool{}
	for i, r := range rules {
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("rule[%d].id is required", i)
		}
		if seen[r.ID] {
			return fmt.Errorf("rule[%d].id %q is not unique", i, r.ID)
		}
		seen[r.ID] = true
		if !validDispositions()[r.Disposition] {
			return fmt.Errorf("rule[%d] (%s): disposition must be 'allow','deny','ask' (got %q)", i, r.ID, r.Disposition)
		}
		if !validSensitivities()[r.Sensitivity] {
			return fmt.Errorf("rule[%d] (%s): sensitivity must be 'low','normal','sensitive' (got %q)", i, r.ID, r.Sensitivity)
		}
		if !validTools()[r.Action.Tool] {
			return fmt.Errorf("rule[%d] (%s): action.tool must be 'bash','write','edit','web','email','*' (got %q)", i, r.ID, r.Action.Tool)
		}
		if _, err := globToRegex(r.Action.Match); err != nil {
			return fmt.Errorf("rule[%d] (%s): invalid action.match glob: %w", i, r.ID, err)
		}
	}
	return nil
}

// ------------------------------------------------------------------
// Matching semantics (v1, deterministic, documented)
//
// A rule's `match` (when present) matches a tool call when EITHER of
// two legs fires. These are the ONLY two legs; a rule with no
// `match` matches every call of its tool:
//
//  1. RENDERED-LINE GLOB — the tool input is rendered as `field=value`
//     lines (RenderInput: top-level fields sorted lexicographically;
//     string values unquoted, every other value compact JSON). The
//     match glob is ANCHORED and matched against EACH rendered line;
//     one hit is enough. Glob dialect (globToRegex): `*` = any
//     sequence of characters, INCLUDING `/` (path.Match semantics
//     with `*` widened across path separators, so `path=/etc/**`
//     matches the line `path=/etc/passwd` AND `path=/etc/cron.d/x`);
//     `?` = exactly one character; every other character literal.
//  2. BASH RAW-COMMAND SUBSTRING (bash only) — when the tool is
//     `bash` and the input has a string `command` field, the match
//     string must occur as a LITERAL SUBSTRING of the raw command
//     (no glob interpretation on this leg: `*`/`?` are literal
//     characters here). This is the documented acceptance
//     semantics: a global policy with a deny rule
//     `{"action":{"tool":"bash","match":"rm -rf"}}` blocks
//     `sudo rm -rf /tmp/x` because "rm -rf" occurs inside the raw
//     command, even though the rendered `command=...` line does not
//     START with it (the line-glob leg is anchored).
//
// One rule, two legs, no ambiguity: leg 1 is anchored-glob-over-each-
// rendered-field-line, leg 2 is literal-substring-over-the-raw-command
// (bash only). Nothing else is consulted.
// ------------------------------------------------------------------

// jsonMarshalCompact is json.Marshal for non-string values (compact
// JSON). A nil encode error yields "null" — never happens for
// JSON-decoded values.
func jsonMarshalCompact(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// globToRegex compiles glob into a fully-anchored Go regexp string.
// It is matched per rendered line WITHOUT (?s), so `.` never spans
// newlines and one line cannot bleed into the next.
func globToRegex(glob string) (string, error) {
	if glob == "" {
		return "", nil // absence of a match = match-all (caller decides)
	}
	if len(glob) > maxGlobLen {
		return "", fmt.Errorf("glob exceeds %d characters", maxGlobLen)
	}
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	// Compile as a sanity check (cannot fail after escaping, but keep
	// the parse gate real and cheap).
	if _, err := regexp.Compile(b.String()); err != nil {
		return "", fmt.Errorf("glob %q does not parse: %w", glob, err)
	}
	return b.String(), nil
}

// RenderInput canonicalizes tool input for matching + hashing.
// See the "Matching semantics" comment above.
func RenderInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	// Sort for determinism (json.Marshal sorts maps already, but the
	// rendered block is also matched against, so be explicit).
	sortStrings(keys)
	var lines []string
	for _, k := range keys {
		v := input[k]
		if s, ok := v.(string); ok {
			lines = append(lines, k+"="+s)
		} else {
			b := jsonMarshalCompact(v)
			lines = append(lines, k+"="+string(b))
		}
	}
	return strings.Join(lines, "\n")
}

// sortStrings sorts s in place (lexicographic, byte order).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// MatchReport documents which leg fired; used by the evaluator to
// build transcript-ready reasons.
type MatchReport struct {
	Matched bool   // the rule's tool+match applies to (tool, input)
	Leg     string // "tool" (no match) | "line" | "raw" | "line+raw"
}

// MatchRule reports whether action's tool+match apply to
// (tool, input), per the documented two-leg semantics. When
// action.Match is empty, only the tool is checked (Leg "tool").
func MatchRule(action RuleAction, tool string, input map[string]any) MatchReport {
	if action.Tool != "*" && action.Tool != tool {
		return MatchReport{}
	}
	if action.Match == "" {
		return MatchReport{Matched: true, Leg: "tool"}
	}
	regex, err := globToRegex(action.Match)
	if err != nil {
		return MatchReport{} // unparsable (should be impossible after validation)
	}
	re := regexp.MustCompile(regex)

	// Leg 1: anchored glob over each rendered field=value line.
	lineHit := false
	for _, line := range strings.Split(RenderInput(input), "\n") {
		if line != "" && re.MatchString(line) {
			lineHit = true
			break
		}
	}

	// Leg 2: literal substring over the raw bash command.
	rawHit := false
	if tool == "bash" {
		if cmd, ok := input["command"].(string); ok && strings.Contains(cmd, action.Match) {
			rawHit = true
		}
	}

	if !lineHit && !rawHit {
		return MatchReport{}
	}
	switch {
	case lineHit && rawHit:
		return MatchReport{Matched: true, Leg: "line+raw"}
	case lineHit:
		return MatchReport{Matched: true, Leg: "line"}
	default:
		return MatchReport{Matched: true, Leg: "raw"}
	}
}
