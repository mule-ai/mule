// Package policy is mule's policy engine (Herd H3.5, migration
// 0015): the control plane that keeps autonomous agent action
// trustworthy. Policies are versioned, owner-scoped bundles of
// deterministic rules; the Evaluator answers (agent_id, tool, input)
// with a synchronous verdict. Forge's before_tool hook is the
// enforcement point; ranch is the approval UI. See spec.md
// "Policy Engine" for the full contract.
package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

// Policy is one row of the policies table.
type Policy struct {
	ID        string
	OwnerID   string
	Scope     string // 'global' | 'agent' | 'org'
	AgentID   string // set for scope='agent'
	Rules     []validation.Rule
	Enabled   bool
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Verdict is the evaluator's answer to a tool call.
//
//	Verdict:  'allow' | 'deny' | 'ask'. The non-blocking default when no
//	          policy matches is 'allow' (rule_id "", reason
//	          "no matching policy").
//	RuleID:   the winning rule's id ("no matching policy" => "").
//	Reason:   a transcript-ready string — the blocked/asked action's
//	          reason must be visible in the agent transcript (e.g.
//	          `rule "rm-guard" denies bash: rm -rf is blocked by
//	          operator policy`).
//	Memoized: true when the verdict came from policy_memo rather than a
//	          fresh rule evaluation.
//	Review:   "pending" when the winning rule is a reviewer:true
//	          (review_mode "agent") rule: the deterministic verdict is
//	          the FLOOR and is returned immediately in v1; the
//	          review-agent upgrade path is a documented follow-up and
//	          forge never blocks on it.
type Verdict struct {
	Verdict  string `json:"verdict"`
	RuleID   string `json:"rule_id"`
	Reason   string `json:"reason"`
	Memoized bool   `json:"memoized"`
	Review   string `json:"review,omitempty"`
}

// verdictRank orders verdicts by restrictiveness: the most restrictive
// verdict wins every conflict (memo lookup, rule conflicts within a
// scope). deny > ask > allow.
func verdictRank(v string) int {
	switch v {
	case "deny":
		return 3
	case "ask":
		return 2
	case "allow":
		return 1
	default:
		return 0
	}
}

// moreRestrictive reports whether v wins a conflict against other.
func moreRestrictive(v, other string) bool {
	return verdictRank(v) > verdictRank(other)
}

// HashAction is the action-hash the memo table is keyed by:
//
//	sha256 hex of  tool + "\n" + canonicalized input

// The canonicalized input is `json.Marshal` of the input map (Go's
// encoding/json sorts map keys, compact output). A nil/empty input
// canonicalizes to "null", so the hash is defined for every call.
// The hash is over the SEMANTIC input, not the rendered matching
// block: two calls with the same tool + same fields hash identically
// regardless of field order or JSON whitespace.
func HashAction(tool string, input map[string]any) string {
	var canonical string
	b, err := json.Marshal(input)
	if err != nil || len(b) == 0 {
		canonical = "null"
	} else {
		canonical = string(b)
	}
	sum := sha256.Sum256([]byte(tool + "\n" + canonical))
	return hex.EncodeToString(sum[:])
}

// Evaluator answers (agent_id, tool, input) with a synchronous
// deterministic verdict (migration 0015, Herd H3.5).
//
// Decision order:
//  1. MEMO FIRST: all policy_memo rows for (agent_id, action_hash).
//     When any exist, the most-restrictive memoized verdict wins and
//     the verdict is memoized=true. The lookup is rule-agnostic (the
//     memoed rule_id rides along but is not a filter), so a deny
//     memoed under one rule still covers the action after rule churn.
//  2. ACTIVE POLICIES: enabled policies where (owner_id = the caller's
//     owner OR scope='global'); scope='agent' policies apply only when
//     their agent_id equals the evaluated agent. (Caller = the ctx
//     principal; admin/background callers see all owners.)
//  3. MOST-SPECIFIC-WINS: agent > org > global. Within a scope, every
//     matching rule is collected and the most-restrictive disposition
//     wins (deny > ask > allow); on a full tie the earliest rule id
//     (as stored in the policy) wins deterministically.
//  4. NO MATCHING POLICY: non-blocking default allow, rule_id "",
//     reason "no matching policy" — and NOTHING is memoized (the
//     default is not a decision; a policy created later must take
//     effect immediately).
//
// Memo writes: the winning verdict is upserted into policy_memo
// (ON CONFLICT DO UPDATE — idempotent, last deterministic evaluation
// wins) EXCEPT:
//   - the no-matching-policy default (above), and
//   - `ask` verdicts: an ask is a pending question, not a decided
//     answer. v1 records the "human said yes" memo in the H2.5 forge
//     hook path (the same endpoint the hook calls back through); the
//     deterministic evaluator itself never memoizes asks, so an
//     unapproved ask re-asks on retry.
//
// reviewer: true rules (v1 DETERMINISTIC FLOOR): the deterministic
// verdict is returned immediately and Verdict.Review is "pending";
// the review-agent upgrade path is documented in spec.md and NOT
// built in v1.
type Evaluator struct {
	db *sql.DB
}

// NewEvaluator builds an Evaluator over the mule Postgres pool.
func NewEvaluator(db *sql.DB) *Evaluator { return &Evaluator{db: db} }

// Evaluate runs the decision order above. See the Evaluator doc for
// the full contract.
func (e *Evaluator) Evaluate(ctx context.Context, agentID, tool string, input map[string]any) (Verdict, error) {
	hash := HashAction(tool, input)

	// 1. Memo first.
	rows, err := e.db.QueryContext(ctx,
		`SELECT rule_id, verdict, reason FROM policy_memo
		  WHERE agent_id = $1 AND action_hash = $2`, agentID, hash)
	if err != nil {
		return Verdict{}, fmt.Errorf("memo lookup: %w", err)
	}
	best := ""
	bestRule, bestReason := "", ""
	for rows.Next() {
		var rule, verdict string
		var reason sql.NullString
		if err := rows.Scan(&rule, &verdict, &reason); err != nil {
			rows.Close()
			return Verdict{}, fmt.Errorf("memo scan: %w", err)
		}
		if verdictRank(verdict) > verdictRank(best) {
			best, bestRule, bestReason = verdict, rule, reason.String
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Verdict{}, fmt.Errorf("memo scan: %w", err)
	}
	rows.Close()
	if best != "" {
		return Verdict{
			Verdict:  best,
			RuleID:   bestRule,
			Reason:   bestReason,
			Memoized: true,
		}, nil
	}

	// 2. Active policies.
	policies, err := e.activePolicies(ctx, agentID)
	if err != nil {
		return Verdict{}, err
	}

	// 3. Most-specific-wins: agent > org > global.
	type scopeWinner struct {
		verdict, ruleID, reason string
		reviewPending           bool
	}
	var winner *scopeWinner
	for _, scope := range []string{validation.ScopeAgent, validation.ScopeOrg, validation.ScopeGlobal} {
		var bestVerdict, bestRule, bestReason string
		bestReview := false
		found := false
		for i := range policies {
			p := &policies[i]
			if p.Scope != scope {
				continue
			}
			for _, rule := range p.Rules {
				if !validation.MatchRule(rule.Action, tool, input).Matched {
					continue
				}
				if !found || moreRestrictive(rule.Disposition, bestVerdict) ||
					(verdictRank(rule.Disposition) == verdictRank(bestVerdict) && rule.ID < bestRule) {
					found = true
					bestVerdict = rule.Disposition
					bestRule = rule.ID
					bestReason = ruleReason(rule)
					bestReview = rule.Reviewer && rule.ReviewMode == "agent"
				}
			}
		}
		if found {
			winner = &scopeWinner{bestVerdict, bestRule, bestReason, bestReview}
			break // most specific scope with any matching rule wins
		}
	}

	if winner == nil {
		// 4. Non-blocking default; nothing memoized.
		return Verdict{Verdict: validation.DispositionAllow, RuleID: "", Reason: "no matching policy"}, nil
	}

	v := Verdict{
		Verdict: winner.verdict,
		RuleID:  winner.ruleID,
		Reason:  winner.reason,
		Review:  "",
	}
	if winner.reviewPending {
		v.Review = "pending" // v1 deterministic floor; see spec.md
	}

	// Memo writes: allow + deny only; ask and the default stay live.
	if v.Verdict == validation.DispositionAllow || v.Verdict == validation.DispositionDeny {
		if err := upsertMemo(ctx, e.db, agentID, v.RuleID, hash, v.Verdict, v.Reason); err != nil {
			return Verdict{}, err
		}
	}
	return v, nil
}

// activePolicies returns enabled policies that may apply to the
// evaluated agent under the caller's ownership: same-owner policies of
// any scope, plus any global-scope policy. Agent-scope policies are
// additionally filtered to the evaluated agent.
func (e *Evaluator) activePolicies(ctx context.Context, agentID string) ([]Policy, error) {
	var ownerClause string
	var args []interface{}
	if p, ok := primitive.PrincipalFrom(ctx); ok && !p.Admin && p.UserID != "" {
		ownerClause = " AND (owner_id = $1 OR scope = 'global')"
		args = append(args, p.UserID)
	}
	q := `SELECT id, owner_id, scope, agent_id, rules, enabled, version
		   FROM policies WHERE enabled = TRUE` + ownerClause +
		` AND (scope <> 'agent' OR agent_id = $` + fmt.Sprint(len(args)+1) + `)`
	args = append(args, agentID)
	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("policy query: %w", err)
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		var p Policy
		var agentID sql.NullString
		var rulesJSON []byte
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Scope, &agentID, &rulesJSON, &p.Enabled, &p.Version); err != nil {
			return nil, fmt.Errorf("policy scan: %w", err)
		}
		p.AgentID = agentID.String
		if err := json.Unmarshal(rulesJSON, &p.Rules); err != nil {
			return nil, fmt.Errorf("policy %s: bad rules JSON: %w", p.ID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("policy scan: %w", err)
	}
	return out, nil
}

// upsertMemo writes/refreshes one policy_memo row for
// (rule_id, agent_id, action_hash). ON CONFLICT DO UPDATE makes it
// idempotent; the most recent write wins. This is the single SQL path
// for memo rows: the deterministic evaluator (above) and the
// public RecordMemo / POST /policies/memo endpoint both call it, so
// the two can't drift.
func upsertMemo(ctx context.Context, db *sql.DB, agentID, ruleID, hash, verdict, reason string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO policy_memo (rule_id, agent_id, action_hash, verdict, reason)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (rule_id, agent_id, action_hash)
		 DO UPDATE SET verdict = EXCLUDED.verdict, reason = EXCLUDED.reason`,
		ruleID, agentID, hash, verdict, reason)
	if err != nil {
		return fmt.Errorf("memo write: %w", err)
	}
	return nil
}

// ruleReason builds the transcript-ready reason for a winning rule:
// the operator's reason when set, else the rule id + description +
// the matched tool, so a blocked/asked action always carries a human
// legible "why" into the transcript.
func ruleReason(rule validation.Rule) string {
	if rule.Reason != "" {
		return rule.Reason
	}
	base := fmt.Sprintf("rule %q (%s): %s", rule.ID, rule.Action.Tool, rule.Disposition)
	if rule.Description != "" {
		base += " — " + rule.Description
	}
	return base
}
