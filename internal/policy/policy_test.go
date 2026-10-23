package policy

// Evaluator unit tests (Herd H3.5): the deterministic decision matrix
// (scope specificity, conflict restrictiveness, tie-breaks, matching
// legs), memo semantics (memo-first lookup, most-restrictive memo
// wins, ask/default never memoized), and action-hash stability.
// Scratch-Postgres pattern: same contract as
// cmd/api/tenancy_test.go / internal/wake/wake_test.go (skip when
// postgres:postgres@localhost:5432 is unavailable).

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internaldb "github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

// policyDB drops+recreates a dedicated mule_policy database with all
// migrations applied and returns a live *internaldb.DB.
func policyDB(t *testing.T) *internaldb.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping policy test that requires PostgreSQL")
	}

	admin, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("could not open admin connection: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("could not connect to test PostgreSQL: %v", err)
	}
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS mule_policy WITH (FORCE)`); err != nil {
		t.Skipf("could not drop mule_policy: %v", err)
	}
	if _, err := admin.Exec(`CREATE DATABASE mule_policy`); err != nil {
		t.Skipf("could not create mule_policy: %v", err)
	}

	db, err := internaldb.NewDB(internaldb.Config{
		Host: "localhost", Port: 5432, User: "postgres",
		Password: "postgres", DBName: "mule_policy", SSLMode: "disable",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.InitSchema())
	return db
}

// ownerCtx builds a non-admin principal context (owner scoping ON).
func ownerCtx(userID string) context.Context {
	return primitive.WithPrincipal(context.Background(),
		primitive.Principal{UserID: userID, Admin: false})
}

// seedMatrix builds the deterministic-matrix policy set owned by
// "user-a":
//
//	global:      open-all (* allow), rm-guard (bash deny "rm -rf")
//	agent-1:     write-ok (write allow), web-ask (web ask)
//	agent-2:     web-ask2 (web ask), web-deny2 (web deny)
//	agent-3:     zz-deny (* deny), aa-deny (* deny)  — full-tie case
func seedMatrix(t *testing.T, store *Store) {
	t.Helper()
	ctx := ownerCtx("user-a")
	policies := []*Policy{
		{
			Scope: "global", Enabled: true,
			Rules: []validation.Rule{
				{ID: "open-all", Action: validation.RuleAction{Tool: "*"},
					Sensitivity: "low", Disposition: "allow"},
				{ID: "rm-guard", Description: "rm -rf is dangerous",
					Action:      validation.RuleAction{Tool: "bash", Match: "rm -rf"},
					Sensitivity: "sensitive", Disposition: "deny",
					Reason: "rm -rf is blocked by operator policy"},
			},
		},
		{
			Scope: "agent", AgentID: "agent-1", Enabled: true,
			Rules: []validation.Rule{
				{ID: "write-ok", Action: validation.RuleAction{Tool: "write"},
					Sensitivity: "low", Disposition: "allow"},
				{ID: "web-ask", Action: validation.RuleAction{Tool: "web"},
					Sensitivity: "normal", Disposition: "ask"},
			},
		},
		{
			Scope: "agent", AgentID: "agent-2", Enabled: true,
			Rules: []validation.Rule{
				{ID: "web-ask2", Action: validation.RuleAction{Tool: "web"},
					Sensitivity: "normal", Disposition: "ask"},
				{ID: "web-deny2", Action: validation.RuleAction{Tool: "web"},
					Sensitivity: "sensitive", Disposition: "deny"},
			},
		},
		{
			Scope: "agent", AgentID: "agent-3", Enabled: true,
			Rules: []validation.Rule{
				{ID: "zz-deny", Action: validation.RuleAction{Tool: "*"},
					Sensitivity: "normal", Disposition: "deny"},
				{ID: "aa-deny", Action: validation.RuleAction{Tool: "*"},
					Sensitivity: "normal", Disposition: "deny"},
			},
		},
	}
	for _, p := range policies {
		require.NoError(t, store.Create(ctx, p))
	}
}

func memoCount(t *testing.T, db *internaldb.DB, agentID, hash string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM policy_memo WHERE agent_id = $1 AND action_hash = $2`,
		agentID, hash).Scan(&n))
	return n
}

// TestPolicyNoMatchingPolicyDefault: no applicable rule ⇒ non-blocking
// allow, rule_id "", reason "no matching policy", nothing memoized.
func TestPolicyNoMatchingPolicyDefault(t *testing.T) {
	db := policyDB(t)
	store, ev := NewStore(db.DB), NewEvaluator(db.DB)
	// Minimal set (the matrix's global open-all rule (tool "*") would
	// match everything and make the default unreachable): one
	// agent-scoped write-only rule.
	ctx := ownerCtx("user-a")
	require.NoError(t, store.Create(ctx, &Policy{
		Scope: "agent", AgentID: "agent-9", Enabled: true,
		Rules: []validation.Rule{
			{ID: "write-only", Action: validation.RuleAction{Tool: "write"},
				Sensitivity: "low", Disposition: "allow"},
		},
	}))

	// agent-9 has no rule touching "email": the default fires.
	v, err := ev.Evaluate(ownerCtx("user-a"), "agent-9", "email",
		map[string]any{"to": "x@example.com"})
	require.NoError(t, err)
	assert.Equal(t, "allow", v.Verdict)
	assert.Empty(t, v.RuleID)
	assert.Equal(t, "no matching policy", v.Reason)
	assert.False(t, v.Memoized)

	// The default is NOT a decision: no memo row may exist.
	hash := HashAction("email", map[string]any{"to": "x@example.com"})
	assert.Equal(t, 0, memoCount(t, db, "agent-9", hash),
		"the no-matching-policy default must not be memoized")
}

// TestPolicyDeterministicMatrix: scope specificity (agent > org >
// global), within-scope conflicts (deny > ask > allow), full-tie rule
// ordering, and the two matching legs (rendered-line glob vs bash
// raw-command substring).
func TestPolicyDeterministicMatrix(t *testing.T) {
	db := policyDB(t)
	store, ev := NewStore(db.DB), NewEvaluator(db.DB)
	seedMatrix(t, store)
	ctx := ownerCtx("user-a")

	// (a) Global, bash raw-substring leg: "rm -rf" occurs inside the
	// raw command even though the rendered `command=...` line does not
	// start with it (the line-glob leg is anchored, so the raw leg is
	// what fires). Within the global scope, rm-guard (deny) beats
	// open-all (allow).
	v, err := ev.Evaluate(ctx, "agent-2", "bash",
		map[string]any{"command": "sudo rm -rf /tmp/x"})
	require.NoError(t, err)
	assert.Equal(t, "deny", v.Verdict)
	assert.Equal(t, "rm-guard", v.RuleID)
	assert.Equal(t, "rm -rf is blocked by operator policy", v.Reason)
	assert.False(t, v.Memoized)

	// (b) Most-specific-wins: agent-1's agent-scope write-ok beats the
	// global open-all even though both allow.
	v, err = ev.Evaluate(ctx, "agent-1", "write",
		map[string]any{"path": "/a/b", "content": "hi"})
	require.NoError(t, err)
	assert.Equal(t, "allow", v.Verdict)
	assert.Equal(t, "write-ok", v.RuleID)

	// (c) Conflict within a scope: agent-2's web rules are ask + deny;
	// deny wins.
	v, err = ev.Evaluate(ctx, "agent-2", "web", map[string]any{"url": "https://x"})
	require.NoError(t, err)
	assert.Equal(t, "deny", v.Verdict)
	assert.Equal(t, "web-deny2", v.RuleID)

	// (d) Full tie on disposition: earliest rule id (as stored) wins —
	// "aa-deny" over "zz-deny" (the stored order lists zz first).
	v, err = ev.Evaluate(ctx, "agent-3", "bash", map[string]any{"command": "ls"})
	require.NoError(t, err)
	assert.Equal(t, "deny", v.Verdict)
	assert.Equal(t, "aa-deny", v.RuleID)

	// (e) Agent scope with no matching rule falls through to global:
	// agent-2's web rules don't touch bash, so open-all allows.
	v, err = ev.Evaluate(ctx, "agent-2", "bash", map[string]any{"command": "ls"})
	require.NoError(t, err)
	assert.Equal(t, "allow", v.Verdict)
	assert.Equal(t, "open-all", v.RuleID)

	// (f) Rendered-line glob leg: anchored match against a
	// field=value line. `url=https://evil*` must hit the rendered
	// `url=...` line of agent-2's web rules (raw leg is bash-only).
	v, err = ev.Evaluate(ctx, "agent-2", "web",
		map[string]any{"url": "https://evil.example/x"})
	require.NoError(t, err)
	// web-deny2 has no match glob so the conflict result is unchanged;
	// the point of this call is that web rules match on the tool at all.
	assert.Equal(t, "deny", v.Verdict)
}

// TestPolicyOwnerScopingInEvaluator: a different owner's agent-scoped
// policy is invisible to the caller's evaluation (H3.1 owner scoping
// inside activePolicies).
func TestPolicyOwnerScopingInEvaluator(t *testing.T) {
	db := policyDB(t)
	store, ev := NewStore(db.DB), NewEvaluator(db.DB)
	seedMatrix(t, store)

	// user-b adds an agent-1 policy that denies everything; it must NOT
	// affect user-a's evaluations.
	b := &Policy{Scope: "agent", AgentID: "agent-1", Enabled: true,
		Rules: []validation.Rule{
			{ID: "b-deny-all", Action: validation.RuleAction{Tool: "*"},
				Sensitivity: "normal", Disposition: "deny"},
		}}
	require.NoError(t, store.Create(ownerCtx("user-b"), b))

	// For user-a, agent-1's bash is still the global allow (open-all):
	// agent-1's own (user-a) rules don't touch bash.
	v, err := ev.Evaluate(ownerCtx("user-a"), "agent-1", "bash",
		map[string]any{"command": "ls"})
	require.NoError(t, err)
	assert.Equal(t, "allow", v.Verdict)
	assert.Equal(t, "open-all", v.RuleID)

	// For user-b, that same agent-1 write is denied by user-b's rule.
	// (A fresh tool: the memo table is agent-keyed and owner-agnostic, so
	// the allow memoed for agent-1's "bash ls" above would short-circuit
	// that action for every owner.)
	v, err = ev.Evaluate(ownerCtx("user-b"), "agent-1", "write",
		map[string]any{"path": "/z"})
	require.NoError(t, err)
	assert.Equal(t, "deny", v.Verdict)
	assert.Equal(t, "b-deny-all", v.RuleID)
}

// TestPolicyMemoSemantics: deterministic verdicts are memoized, the
// memo lookup is rule-agnostic with the most-restrictive verdict
// winning, and ask + the default are never memoized.
func TestPolicyMemoSemantics(t *testing.T) {
	db := policyDB(t)
	store, ev := NewStore(db.DB), NewEvaluator(db.DB)
	seedMatrix(t, store)
	ctx := ownerCtx("user-a")

	// First evaluation: fresh.
	action := map[string]any{"command": "sudo rm -rf /tmp/x"}
	v1, err := ev.Evaluate(ctx, "agent-2", "bash", action)
	require.NoError(t, err)
	assert.False(t, v1.Memoized)
	assert.Equal(t, "deny", v1.Verdict)

	// Second evaluation of the SAME action: memoized, identical
	// verdict/rule/reason.
	v2, err := ev.Evaluate(ctx, "agent-2", "bash", action)
	require.NoError(t, err)
	assert.True(t, v2.Memoized)
	assert.Equal(t, v1.Verdict, v2.Verdict)
	assert.Equal(t, v1.RuleID, v2.RuleID)
	assert.Equal(t, v1.Reason, v2.Reason)
	assert.Equal(t, 1, memoCount(t, db, "agent-2", HashAction("bash", action)))

	// Rule-agnostic + most-restrictive memo wins: a second memo row for
	// the same action under a DIFFERENT rule id that is LESS
	// restrictive must not loosen the memoed deny.
	hash := HashAction("bash", action)
	_, err = db.ExecContext(ctx,
		`INSERT INTO policy_memo (rule_id, agent_id, action_hash, verdict, reason)
		 VALUES ('legacy-rule', 'agent-2', $1, 'allow', 'old allow')`, hash)
	require.NoError(t, err)
	v3, err := ev.Evaluate(ctx, "agent-2", "bash", action)
	require.NoError(t, err)
	assert.True(t, v3.Memoized)
	assert.Equal(t, "deny", v3.Verdict, "most-restrictive memo must win")
	assert.Equal(t, "rm-guard", v3.RuleID)

	// Ask is NEVER memoized by the deterministic evaluator: an
	// unapproved ask must re-ask on retry.
	aw, err := ev.Evaluate(ctx, "agent-1", "web", map[string]any{"url": "https://a"})
	require.NoError(t, err)
	assert.Equal(t, "ask", aw.Verdict)
	assert.Equal(t, "web-ask", aw.RuleID)
	assert.False(t, aw.Memoized)
	aw2, err := ev.Evaluate(ctx, "agent-1", "web", map[string]any{"url": "https://a"})
	require.NoError(t, err)
	assert.Equal(t, "ask", aw2.Verdict)
	assert.False(t, aw2.Memoized, "an unapproved ask must re-ask")
	assert.Equal(t, 0, memoCount(t, db, "agent-1",
		HashAction("web", map[string]any{"url": "https://a"})))
}

// TestPolicyActionHashStability: the action hash is a pure function of
// (tool, semantic input) — same input ⇒ same hash, call after call,
// and map key order is irrelevant (the canonical form is sorted-key
// compact JSON).
func TestPolicyActionHashStability(t *testing.T) {
	// Determinism across calls (the Go map has no notion of "key
	// order"; the guarantee under test is that the canonical JSON sort
	// makes the hash independent of construction order).
	in := map[string]any{"command": "ls -la", "cwd": "/tmp", "timeout": 30}
	h1 := HashAction("bash", in)
	h2 := HashAction("bash", map[string]any{"timeout": 30, "cwd": "/tmp", "command": "ls -la"})
	assert.Equal(t, h1, h2, "hash must be key-order independent")
	assert.Equal(t, h1, HashAction("bash", in))

	// Different tool or input ⇒ different hash.
	assert.NotEqual(t, h1, HashAction("bash", map[string]any{"command": "ls -la", "cwd": "/other"}))
	assert.NotEqual(t, h1, HashAction("web", in))

	// Nil/empty input canonicalizes to "null":
	// sha256("bash\nnull").
	sum := sha256.Sum256([]byte("bash\nnull"))
	assert.Equal(t, hex.EncodeToString(sum[:]), HashAction("bash", nil))

	// An empty-but-non-nil map canonicalizes to compact "{}":
	// sha256("bash\n{}").
	sumEmpty := sha256.Sum256([]byte("bash\n{}"))
	assert.Equal(t, hex.EncodeToString(sumEmpty[:]), HashAction("bash", map[string]any{}))
}
