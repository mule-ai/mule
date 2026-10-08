package main

// Policy engine API (Herd H3.5): owner-scoped CRUD over mule's
// control plane (internal/policy, migration 0015) plus the
// synchronous evaluate endpoint that forge's before_tool hook calls
// and the agent-proposal flow.
//
// Auth choice (documented in spec.md "Policy Engine"):
//   - CRUD (GET/POST/PATCH/DELETE /api/v1/policies[/{id}]) follows the
//     H3.1 restricted class: a restricted (agent) key may READ but not
//     write.
//   - POST /api/v1/policies/evaluate stays OPEN to any valid key
//     (incl. restricted agent keys) — it is the forge hook's query
//     surface: the agent's own key must be able to ask the control
//     plane about its actions. (Exception carved into the H3.1
//     restricted-write check in internal/api/auth.go.)
//   - POST /api/v1/policies/proposals also stays OPEN to restricted
//     keys: agents DRAFT rules here. Materializing a draft requires
//     POST /api/v1/policies/proposals/{id}/approve, which is a
//     restricted-class write (non-restricted key only) — the
//     "agent proposes, human approves" loop.
//
// Evaluate is synchronous and deterministic-only in v1: p99 < 50 ms
// target, no review-agent path (the deterministic verdict of a
// reviewer:true rule is the floor; see spec.md).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/mule-ai/mule/internal/api"
	"github.com/mule-ai/mule/internal/policy"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

// policyAPI is the request/response shape for policy CRUD.
type policyAPI struct {
	ID        string            `json:"id,omitempty"`
	OwnerID   string            `json:"owner_id,omitempty"`
	Scope     string            `json:"scope"`
	AgentID   string            `json:"agent_id,omitempty"`
	Rules     []validation.Rule `json:"rules"`
	Enabled   *bool             `json:"enabled"`
	Version   int               `json:"version,omitempty"`
	CreatedAt *time.Time        `json:"created_at,omitempty"`
	UpdatedAt *time.Time        `json:"updated_at,omitempty"`
}

func toPolicyAPI(p *policy.Policy) policyAPI {
	a := policyAPI{
		ID: p.ID, OwnerID: p.OwnerID, Scope: p.Scope, AgentID: p.AgentID,
		Rules: p.Rules, Version: p.Version,
	}
	if p.Enabled {
		t := true
		a.Enabled = &t
	} else {
		f := false
		a.Enabled = &f
	}
	if !p.CreatedAt.IsZero() {
		c := p.CreatedAt
		a.CreatedAt = &c
	}
	if !p.UpdatedAt.IsZero() {
		u := p.UpdatedAt
		a.UpdatedAt = &u
	}
	return a
}

// listPoliciesHandler: GET /api/v1/policies — the owner's policies.
func (h *apiHandler) listPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	policies, err := h.policyStore.List(r.Context())
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list policies: %w", err), http.StatusInternalServerError)
		return
	}
	out := []policyAPI{}
	for _, p := range policies {
		out = append(out, toPolicyAPI(p))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"policies": out})
}

// createPoliciesHandler: POST /api/v1/policies
//
//	body: {scope, agent_id? (required when scope=agent),
//	         rules: [...], enabled?}
//
// Rules must validate (disposition/sensitivity/tool/glob, unique
// ids, non-empty array) or the request is 400. Writes a 'created'
// audit row in the same transaction.
func (h *apiHandler) createPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	var req policyAPI
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	if req.Scope == "" {
		api.HandleError(w, fmt.Errorf("scope is required ('global' | 'agent' | 'org')"), http.StatusBadRequest)
		return
	}
	if err := validation.ValidatePolicyScope(req.Scope, req.AgentID); err != nil {
		api.HandleError(w, err, http.StatusBadRequest)
		return
	}
	if err := validation.ValidateRules(req.Rules); err != nil {
		api.HandleError(w, err, http.StatusBadRequest)
		return
	}
	p := &policy.Policy{
		Scope:   req.Scope,
		AgentID: req.AgentID,
		Rules:   req.Rules,
		Enabled: true,
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := h.policyStore.Create(r.Context(), p); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create policy: %w", err), http.StatusInternalServerError)
		return
	}
	created, err := h.policyStore.Get(r.Context(), p.ID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to reload policy: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toPolicyAPI(created))
}

// getPolicyHandler: GET /api/v1/policies/{id} (404 when not the
// caller's).
func (h *apiHandler) getPolicyHandler(w http.ResponseWriter, r *http.Request) {
	p, err := h.policyStore.Get(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("policy not found: %s", mux.Vars(r)["id"]), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get policy: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toPolicyAPI(p))
}

// patchPolicyHandler: PATCH /api/v1/policies/{id}
//
//	body (all optional): {scope?, agent_id?, rules?, enabled?}
//
// Any content change bumps version and writes a policy_audit row in
// one transaction (change text names what changed, e.g.
// "rules changed v1→v2"; "disabled v3"). A no-content patch is a
// 200 of the current row WITHOUT a version bump.
func (h *apiHandler) patchPolicyHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	cur, err := h.policyStore.Get(r.Context(), id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("policy not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get policy: %w", err), http.StatusInternalServerError)
		}
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	next := &policy.Policy{
		ID: id, OwnerID: cur.OwnerID,
		Scope: cur.Scope, AgentID: cur.AgentID,
		Rules: cur.Rules, Enabled: cur.Enabled,
	}
	var changed []string
	if v, present := patch["scope"]; present {
		s, ok := v.(string)
		if !ok || s == "" {
			api.HandleError(w, fmt.Errorf("scope must be a non-empty string"), http.StatusBadRequest)
			return
		}
		if s != cur.Scope {
			next.Scope = s
			changed = append(changed, fmt.Sprintf("rescoped to %q", s))
		}
	}
	if v, present := patch["agent_id"]; present {
		s, _ := v.(string) // empty = clear (allowed for non-agent scopes)
		if s != cur.AgentID {
			next.AgentID = s
			changed = append(changed, "agent_id changed")
		}
	}
	if v, present := patch["rules"]; present {
		var rules []validation.Rule
		if err := json.Unmarshal([]byte(mustString(v)), &rules); err != nil {
			api.HandleError(w, fmt.Errorf("rules must be an array of policy rules: %w", err), http.StatusBadRequest)
			return
		}
		if !rulesEqual(rules, cur.Rules) {
			next.Rules = rules
			changed = append(changed, fmt.Sprintf("rules changed v%d→v%d", cur.Version, cur.Version+1))
		}
	}
	if v, present := patch["enabled"]; present {
		b, ok := v.(bool)
		if !ok {
			api.HandleError(w, fmt.Errorf("enabled must be a boolean"), http.StatusBadRequest)
			return
		}
		if b != cur.Enabled {
			next.Enabled = b
			if b {
				changed = append(changed, fmt.Sprintf("enabled v%d", cur.Version+1))
			} else {
				changed = append(changed, fmt.Sprintf("disabled v%d", cur.Version+1))
			}
		}
	}

	if len(changed) == 0 {
		// No-op patch: 200 of the current row, no version bump.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toPolicyAPI(cur))
		return
	}

	updated, err := h.policyStore.Set(r.Context(), id, next, joinChanges(changed))
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("policy not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update policy: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toPolicyAPI(updated))
}

// deletePolicyHandler: DELETE /api/v1/policies/{id} — 204; the
// 'deleted' audit row survives via policy_audit.policy_id
// ON DELETE SET NULL.
func (h *apiHandler) deletePolicyHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.policyStore.Delete(r.Context(), id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("policy not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete policy: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// evaluatePolicyHandler: POST /api/v1/policies/evaluate — the forge
// before_tool hook's synchronous query surface. Open to ANY valid
// key (incl. restricted agent keys); see the package doc at the top
// of this file.
//
//	body: {agent_id, tool, input?}
//	response: {verdict: allow|deny|ask, rule_id, reason,
//	          memoized, review?}
func (h *apiHandler) evaluatePolicyHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string                 `json:"agent_id"`
		Tool    string                 `json:"tool"`
		Input   map[string]interface{} `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	if req.AgentID == "" || req.Tool == "" {
		api.HandleError(w, fmt.Errorf("agent_id and tool are required"), http.StatusBadRequest)
		return
	}
	verdict, err := h.policyEvaluator.Evaluate(r.Context(), req.AgentID, req.Tool, req.Input)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to evaluate policy: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(verdict)
}

// createProposalHandler: POST /api/v1/policies/proposals — an agent
// (or operator) drafts a new policy. Open to restricted keys (the
// agent-draft side of the loop); materialization requires approval.
//
//	body: {draft: {scope, agent_id?, rules: [...]}}
//	response: {proposal_id}
func (h *apiHandler) createProposalHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Draft json.RawMessage `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	if len(req.Draft) == 0 {
		api.HandleError(w, fmt.Errorf("draft is required"), http.StatusBadRequest)
		return
	}
	draft, err := policy.ParseDraft(req.Draft)
	if err != nil {
		api.HandleError(w, err, http.StatusBadRequest)
		return
	}
	proposalID, err := h.policyStore.RecordProposal(r.Context(), draft)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to record proposal: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"proposal_id": proposalID})
}

// approveProposalHandler: POST /api/v1/policies/proposals/{id}/approve
// — the HUMAN side of the loop; non-restricted keys only (the H3.1
// restricted-class check enforces this at the route). Creates the
// policy (owned by the approver, version 1) and records the
// 'approved-proposal' audit row, in one transaction.
func (h *apiHandler) approveProposalHandler(w http.ResponseWriter, r *http.Request) {
	p, err := h.policyStore.ApproveProposal(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("proposal not found (or already approved): %s", mux.Vars(r)["id"]), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to approve proposal: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toPolicyAPI(p))
}

// mustString renders a decoded-JSON patch value back to bytes for
// re-decoding (rules arrive as a generic JSON value in the patch map).
func mustString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// rulesEqual reports whether two rules arrays are JSON-identical
// (order-sensitive: the stored array is what gets re-serialized).
func rulesEqual(a, b []validation.Rule) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// joinChanges joins audit change fragments with "; ".
func joinChanges(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}
