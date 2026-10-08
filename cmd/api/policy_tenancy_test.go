package main

// Policy engine tenancy tests (Herd H3.5): version bumps + audit rows,
// owner isolation on the evaluate surface, the restricted-key carve-out
// (evaluate + proposals stay open; CRUD + approve are restricted
// class), the agent-proposal / human-approval flow, and request
// validation. Same scratch-Postgres + router contract as
// tenancy_test.go / wake_tenancy_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/api"
	internaldb "github.com/mule-ai/mule/internal/database"
)

// policyAdminBoot seeds the deterministic admin + key used by the
// policy tenancy tests.
func policyAdminBoot(t *testing.T) string {
	t.Helper()
	t.Setenv("MULE_ADMIN_EMAIL", "policyadmin@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "policy-admin-pass")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_2222adminkey000000000000000000")
	t.Setenv("MULE_ADMIN_ID", "")
	return "sk_mule_2222adminkey000000000000000000"
}

// auditRows returns the policy_audit rows for a policy, oldest first.
func auditRows(t *testing.T, db *internaldb.DB, policyID string) []map[string]interface{} {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT id, policy_id, actor, change, prev_version, new_version, detail
		   FROM policy_audit WHERE policy_id = $1 ORDER BY at, id`, policyID)
	require.NoError(t, err)
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, policyID, actor, change string
		var prevV, newV interface{}
		var detail []byte
		require.NoError(t, rows.Scan(&id, &policyID, &actor, &change, &prevV, &newV, &detail))
		out = append(out, map[string]interface{}{
			"id": id, "policy_id": policyID, "actor": actor,
			"change": change, "prev_version": prevV,
			"new_version": newV, "detail": string(detail),
		})
	}
	require.NoError(t, rows.Err())
	return out
}

// TestPoliciesVersionAndAudit: POST → v1 + 'created' audit row;
// content PATCH → v2 + audit row; no-content PATCH → no bump;
// disable → v3 + 'disabled' audit row.
func TestPoliciesVersionAndAudit(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey := policyAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	router := tenancyRouter(t, db)

	// POST → v1.
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/policies", adminKey,
		[]byte(`{"scope":"global","rules":[{"id":"r1","action":{"tool":"bash"},`+
			`"sensitivity":"low","disposition":"allow"}]}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	assert.Equal(t, 1, created.Version)

	// Exactly one audit row: 'created', v0→v1, actor = the admin user.
	rows := auditRows(t, db, created.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "created", rows[0]["change"])
	assert.Equal(t, int64(1), rows[0]["new_version"])
	assert.Nil(t, rows[0]["prev_version"])
	assert.Equal(t, api.DefaultAdminID, rows[0]["actor"])

	// Content PATCH → v2 + audit row.
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/policies/"+created.ID,
		adminKey, []byte(`{"rules":[
			{"id":"r1","action":{"tool":"bash"},"sensitivity":"low","disposition":"allow"},
			{"id":"r2","action":{"tool":"bash","match":"rm -rf"},"sensitivity":"sensitive","disposition":"deny"}
		]}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var patched struct {
		Version int `json:"version"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	assert.Equal(t, 2, patched.Version, "a rules change must bump the version")

	rows = auditRows(t, db, created.ID)
	require.Len(t, rows, 2, "the PATCH must write exactly one more audit row")
	assert.Equal(t, "rules changed v1→v2", rows[1]["change"])
	assert.Equal(t, int64(1), rows[1]["prev_version"])
	assert.Equal(t, int64(2), rows[1]["new_version"])

	// No-content PATCH → 200 of the current row, NO version bump.
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/policies/"+created.ID,
		adminKey, []byte(`{}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	assert.Equal(t, 2, patched.Version, "a no-op patch must not bump the version")
	assert.Len(t, auditRows(t, db, created.ID), 2)

	// Disable → v3 + 'disabled' audit row.
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/policies/"+created.ID,
		adminKey, []byte(`{"enabled":false}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	assert.Equal(t, 3, patched.Version)
	rows = auditRows(t, db, created.ID)
	require.Len(t, rows, 3)
	assert.Equal(t, "disabled v3", rows[2]["change"])
}

// TestPoliciesOwnerIsolation: user A's agent-scoped policy is
// invisible to user B — on the CRUD surface (404/empty) AND on the
// evaluate surface (B gets the no-matching-policy default).
func TestPoliciesOwnerIsolation(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey := policyAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	userB := insertTestUser(t, db, "pol-user-b", "polb@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	// A creates an agent-scoped deny policy.
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/policies", adminKey,
		[]byte(`{"scope":"agent","agent_id":"agent-iso","rules":[
			{"id":"d1","action":{"tool":"bash"},"sensitivity":"sensitive",
			 "disposition":"deny","reason":"no bash for agent-iso"}]}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// B: empty list, 404 by id.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/policies", userB, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"policies":[]`,
		"B's policy list must be empty: %s", w.Body.String())
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/policies/"+created.ID, userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "B must not see A's policy by id")

	// B's evaluate on that agent: A's deny is invisible → default
	// non-blocking allow.
	eval := `{"agent_id":"agent-iso","tool":"bash","input":{"command":"ls"}}`
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/evaluate", userB, []byte(eval))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var bVerdict struct {
		Verdict  string `json:"verdict"`
		RuleID   string `json:"rule_id"`
		Reason   string `json:"reason"`
		Memoized bool   `json:"memoized"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bVerdict))
	assert.Equal(t, "allow", bVerdict.Verdict)
	assert.Empty(t, bVerdict.RuleID)
	assert.Equal(t, "no matching policy", bVerdict.Reason)

	// A's evaluate on the same agent: the deny applies.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/evaluate", adminKey, []byte(eval))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var aVerdict struct {
		Verdict string `json:"verdict"`
		RuleID  string `json:"rule_id"`
		Reason  string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aVerdict))
	assert.Equal(t, "deny", aVerdict.Verdict)
	assert.Equal(t, "d1", aVerdict.RuleID)
	assert.Equal(t, "no bash for agent-iso", aVerdict.Reason)
}

// TestPoliciesRestrictedKey: a restricted (agent) key cannot write
// policy CRUD (403) but CAN call evaluate (the forge hook's query
// surface) and draft proposals (the agent-draft side of the loop);
// approving a proposal stays a restricted-class write.
func TestPoliciesRestrictedKey(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey := policyAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	restricted := insertTestUser(t, db, "pol-user-r", "polr@example.com", true, time.Time{})
	router := tenancyRouter(t, db)

	// Reads are allowed for restricted keys.
	w := tenancyRequest(t, router, http.MethodGet, "/api/v1/policies", restricted, nil)
	assert.Equal(t, http.StatusOK, w.Code, "restricted key must read policies: %s", w.Body.String())

	// CRUD writes are the H3.1 restricted class → 403.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies", restricted,
		[]byte(`{"scope":"global","rules":[{"id":"x","action":{"tool":"bash"},
			"sensitivity":"low","disposition":"allow"}]}`))
	assert.Equal(t, http.StatusForbidden, w.Code,
		"restricted key must not create policies: %s", w.Body.String())
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/policies/any-id", restricted, []byte(`{}`))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = tenancyRequest(t, router, http.MethodDelete, "/api/v1/policies/any-id", restricted, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// evaluate stays OPEN to restricted keys (documented carve-out).
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/evaluate",
		restricted, []byte(`{"agent_id":"agent-r","tool":"bash","input":{"command":"ls"}}`))
	require.Equal(t, http.StatusOK, w.Code,
		"restricted key must be able to evaluate: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"verdict":"allow"`)

	// proposals stay OPEN too (agents draft; humans approve).
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/proposals",
		restricted, []byte(`{"draft":{"scope":"global","rules":[
			{"id":"p1","action":{"tool":"email"},"sensitivity":"sensitive","disposition":"deny"}]}}`))
	require.Equal(t, http.StatusCreated, w.Code,
		"restricted key must be able to propose: %s", w.Body.String())
	var prop struct {
		ProposalID string `json:"proposal_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prop))
	require.NotEmpty(t, prop.ProposalID)

	// ...but approval stays a restricted-class write → 403.
	w = tenancyRequest(t, router, http.MethodPost,
		"/api/v1/policies/proposals/"+prop.ProposalID+"/approve", restricted, nil)
	assert.Equal(t, http.StatusForbidden, w.Code,
		"restricted key must not approve proposals: %s", w.Body.String())

	// The unrestricted admin key can approve the same proposal.
	w = tenancyRequest(t, router, http.MethodPost,
		"/api/v1/policies/proposals/"+prop.ProposalID+"/approve", adminKey, nil)
	assert.Equal(t, http.StatusCreated, w.Code, adminKey+" approve: %s", w.Body.String())
}

// TestPoliciesProposalFlow: POST proposals → audit row (change
// 'proposal', policy_id NULL, draft JSON in detail); POST
// proposals/{id}/approve with a non-restricted key creates the policy
// (owner = approver, v1, enabled) and morphs the row to
// 'approved-proposal'; re-approval 404s.
func TestPoliciesProposalFlow(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey := policyAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	router := tenancyRouter(t, db)

	// Draft the proposal.
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/proposals",
		adminKey, []byte(`{"draft":{"scope":"agent","agent_id":"agent-prop","rules":[
			{"id":"p1","action":{"tool":"web","match":"https://internal*"},
			 "sensitivity":"sensitive","disposition":"deny",
			 "reason":"internal web is off-limits"}]}}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var prop struct {
		ProposalID string `json:"proposal_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prop))
	require.NotEmpty(t, prop.ProposalID)

	// The proposal is an audit row: change 'proposal', policy_id NULL,
	// the draft JSON in detail.
	var change string
	var policyID interface{}
	var detail string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT change, policy_id, detail FROM policy_audit WHERE id = $1`,
		prop.ProposalID).Scan(&change, &policyID, &detail))
	assert.Equal(t, "proposal", change)
	assert.Nil(t, policyID, "a pending proposal must have policy_id NULL")
	assert.Contains(t, detail, `"agent-prop"`)
	assert.Contains(t, detail, `"internal web is off-limits"`)

	// Approve with a non-restricted key: the policy materializes,
	// owned by the APPROVER, version 1, enabled.
	w = tenancyRequest(t, router, http.MethodPost,
		"/api/v1/policies/proposals/"+prop.ProposalID+"/approve", adminKey, nil)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var approved struct {
		ID      string `json:"id"`
		OwnerID string `json:"owner_id"`
		Scope   string `json:"scope"`
		AgentID string `json:"agent_id"`
		Version int    `json:"version"`
		Enabled *bool  `json:"enabled"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &approved))
	assert.Equal(t, api.DefaultAdminID, approved.OwnerID, "the approver owns the new policy")
	assert.Equal(t, "agent", approved.Scope)
	assert.Equal(t, "agent-prop", approved.AgentID)
	assert.Equal(t, 1, approved.Version)
	require.NotNil(t, approved.Enabled)
	assert.True(t, *approved.Enabled)

	// The proposal row morphs into 'approved-proposal' pointing at the
	// new policy; a 'created' audit row exists for the new policy.
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT change, policy_id, new_version FROM policy_audit WHERE id = $1`,
		prop.ProposalID).Scan(&change, &policyID, &approved.Version))
	assert.Equal(t, "approved-proposal", change)
	assert.Equal(t, approved.ID, policyID.(string))

	// The new policy is visible to its owner.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/policies/"+approved.ID, adminKey, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Re-approval 404s (the row is no longer a pending proposal).
	w = tenancyRequest(t, router, http.MethodPost,
		"/api/v1/policies/proposals/"+prop.ProposalID+"/approve", adminKey, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestPoliciesValidation: bad disposition / bad sensitivity /
// agent-scope without agent_id / empty rules / missing scope / bad
// proposal draft are all 400.
func TestPoliciesValidation(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey := policyAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	router := tenancyRouter(t, db)

	validRule := `{"id":"r1","action":{"tool":"bash"},"sensitivity":"low","disposition":"allow"}`
	post := func(body string) int {
		return tenancyRequest(t, router, http.MethodPost, "/api/v1/policies",
			adminKey, []byte(body)).Code
	}

	// Bad disposition.
	w := post(`{"scope":"global","rules":[` +
		`{"id":"b1","action":{"tool":"bash"},"sensitivity":"low","disposition":"maybe"}]}`)
	assert.Equal(t, http.StatusBadRequest, w, "bad disposition must 400")

	// Bad sensitivity.
	w = post(`{"scope":"global","rules":[` +
		`{"id":"b2","action":{"tool":"bash"},"sensitivity":"high","disposition":"allow"}]}`)
	assert.Equal(t, http.StatusBadRequest, w, "bad sensitivity must 400")

	// Agent scope without agent_id.
	w = post(`{"scope":"agent","rules":[` + validRule + `]}`)
	assert.Equal(t, http.StatusBadRequest, w, "agent scope without agent_id must 400")

	// Bad scope.
	w = post(`{"scope":"planet","rules":[` + validRule + `]}`)
	assert.Equal(t, http.StatusBadRequest, w)

	// Empty rules.
	w = post(`{"scope":"global","rules":[]}`)
	assert.Equal(t, http.StatusBadRequest, w)

	// Missing scope.
	w = post(`{"rules":[` + validRule + `]}`)
	assert.Equal(t, http.StatusBadRequest, w)

	// Bad tool.
	w = post(`{"scope":"global","rules":[
		{"id":"b3","action":{"tool":"teleport"},"sensitivity":"low","disposition":"allow"}]}`)
	assert.Equal(t, http.StatusBadRequest, w)

	// Duplicate rule ids.
	w = post(`{"scope":"global","rules":[` + validRule + `,` + validRule + `]}`)
	assert.Equal(t, http.StatusBadRequest, w)

	// Proposal with an invalid draft is also 400.
	assert.Equal(t, http.StatusBadRequest,
		tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/proposals",
			adminKey, []byte(`{"draft":{"scope":"global","rules":[
				{"id":"x","action":{"tool":"bash"},"sensitivity":"low","disposition":"maybe"}]}}`)).Code,
		"bad proposal draft must 400")
}

// TestPoliciesNoAuthMode: MULE_NO_AUTH=1 — the synthetic admin owns
// created policies and the full surface works without a key
// (regression gate, same contract as the other primitive routes).
func TestPoliciesNoAuthMode(t *testing.T) {
	db := tenancyDB(t)
	t.Setenv("MULE_NO_AUTH", "1")
	router := tenancyRouter(t, db)

	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/policies", "",
		[]byte(`{"scope":"global","rules":[
			{"id":"r1","action":{"tool":"bash","match":"rm -rf"},
			 "sensitivity":"sensitive","disposition":"deny"}]}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID      string `json:"id"`
		OwnerID string `json:"owner_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, api.DefaultAdminID, created.OwnerID,
		"synthetic admin must own created policies")

	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/policies/evaluate", "",
		[]byte(`{"agent_id":"agent-1","tool":"bash","input":{"command":"rm -rf /tmp"}}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"verdict":"deny"`)
	assert.Contains(t, w.Body.String(), `"rule_id":"r1"`)

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/policies", "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), created.ID)
}
