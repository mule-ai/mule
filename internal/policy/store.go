package policy

// Policy engine store (Herd H3.5, migration 0015).
//
// A policy is a named, versioned, owner-scoped bundle of deterministic
// rules (types: internal/validation.Rule). This file owns the
// mutations (create / set / delete), the policy_audit trail, and the
// agent-proposal flow; the Evaluator (policy.go) owns verdicts.
//
// Versioning + audit: every mutation bumps version (on updates) and
// writes a policy_audit row in the SAME transaction. The audit row's
// actor is the ctx principal's user id, or 'system' when no principal
// is present. policy_audit.policy_id is ON DELETE SET NULL so the
// 'deleted' row survives the policy's deletion.
//
// Owner scoping follows the H3.1 pattern (primitive.OwnerScope):
// admin principals see all owners; non-admin principals are
// owner-scoped on every read/write; inserts are stamped with the
// caller's owner.
//
// Proposals: agents (restricted keys included) may propose a new
// policy via RecordProposal — a policy_audit row with change='proposal'
// and policy_id NULL whose id IS the proposal id. ApproveProposal
// (non-restricted keys only, enforced by the H3.1 restricted-class
// route check) materializes it: the new policy is owned by the
// APPROVER and the proposal row morphs into 'approved-proposal'.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

// Store is the PostgreSQL store for the policy engine.
type Store struct {
	db *sql.DB
}

// NewStore creates a policy Store over an existing connection pool.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const policyColumns = `id, owner_id, scope, agent_id, rules, enabled, version,
    created_at, updated_at`

func scanPolicy(row interface {
	Scan(dest ...interface{}) error
}) (*Policy, error) {
	p := &Policy{}
	var rulesJSON []byte
	var agentID sql.NullString
	err := row.Scan(
		&p.ID, &p.OwnerID, &p.Scope, &agentID, &rulesJSON,
		&p.Enabled, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if agentID.Valid {
		p.AgentID = agentID.String
	}
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &p.Rules); err != nil {
			return nil, fmt.Errorf("failed to decode policy rules for %s: %w", p.ID, err)
		}
	}
	return p, nil
}

// actor is the audit-log identity for the current context: the caller's
// user id when a principal is present, 'system' otherwise.
func actor(ctx context.Context) string {
	if p, ok := primitive.PrincipalFrom(ctx); ok && p.UserID != "" {
		return p.UserID
	}
	return "system"
}

// auditInsert appends one policy_audit row inside tx. prev/new version
// 0 → NULL.
func auditInsert(tx *sql.Tx, ctx context.Context, policyID, change string, prev, new int, detail []byte) error {
	var pid, prevV, newV interface{}
	if policyID != "" {
		pid = policyID
	}
	if prev > 0 {
		prevV = prev
	}
	if new > 0 {
		newV = new
	}
	// detail stays an untyped nil interface (=> SQL NULL) when empty:
	// lib/pq would encode a typed-nil []byte as the empty string, which
	// Postgres rejects as invalid JSON.
	var detailArg interface{}
	if detail != nil {
		detailArg = detail
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO policy_audit (id, policy_id, actor, change, prev_version, new_version, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New().String(), pid, actor(ctx), change, prevV, newV, detailArg)
	if err != nil {
		return fmt.Errorf("failed to write policy_audit row: %w", err)
	}
	return nil
}

// Create inserts a new policy (version 1) and writes the 'created'
// audit row in one transaction. The owner comes from the ctx
// principal when present, else p.OwnerID must be set.
func (s *Store) Create(ctx context.Context, p *Policy) error {
	if err := validation.ValidatePolicyScope(p.Scope, p.AgentID); err != nil {
		return err
	}
	if err := validation.ValidateRules(p.Rules); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.OwnerID == "" {
		if pr, ok := primitive.PrincipalFrom(ctx); ok && pr.UserID != "" {
			p.OwnerID = pr.UserID
		}
	}
	if p.OwnerID == "" {
		return errors.New("policies require an owner")
	}
	if p.Version == 0 {
		p.Version = 1
	}
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return fmt.Errorf("failed to encode policy rules: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin policy txn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var agentID interface{}
	if p.AgentID != "" {
		agentID = p.AgentID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO policies (id, owner_id, scope, agent_id, rules, enabled, version,
		                      created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
		p.ID, p.OwnerID, p.Scope, agentID, rulesJSON, p.Enabled, p.Version); err != nil {
		return fmt.Errorf("failed to create policy: %w", err)
	}
	if err := auditInsert(tx, ctx, p.ID, "created", 0, p.Version, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit policy create: %w", err)
	}
	return nil
}

// List returns the caller's policies (owner-scoped for non-admin
// principals; admin sees all owners), oldest first.
func (s *Store) List(ctx context.Context) ([]*Policy, error) {
	query := "SELECT " + policyColumns + " FROM policies"
	args := []interface{}{}
	if clause, arg := primitive.OwnerScope(ctx, 1); clause != "" {
		query += " WHERE " + clause
		args = append(args, arg)
	}
	query += " ORDER BY created_at"
	return s.queryPolicies(ctx, query, args...)
}

// Get returns one of the caller's policies by id (ErrNotFound when
// absent or not owned).
func (s *Store) Get(ctx context.Context, id string) (*Policy, error) {
	query := "SELECT " + policyColumns + " FROM policies WHERE id = $1"
	args := []interface{}{id}
	if clause, arg := primitive.OwnerScope(ctx, 2); clause != "" {
		query += " AND " + clause
		args = append(args, arg)
	}
	p, err := scanPolicy(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, primitive.ErrNotFound
	}
	return p, err
}

// Set replaces the mutable fields (scope, agent_id, rules, enabled) of
// an existing policy, bumps version, and writes the audit row in one
// transaction. change is the audit description, computed by the
// handler (e.g. "rules changed v1→v2").
func (s *Store) Set(ctx context.Context, id string, p *Policy, change string) (*Policy, error) {
	if err := validation.ValidatePolicyScope(p.Scope, p.AgentID); err != nil {
		return nil, err
	}
	if err := validation.ValidateRules(p.Rules); err != nil {
		return nil, err
	}
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return nil, fmt.Errorf("failed to encode policy rules: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin policy txn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock + owner-scope the current row.
	getQuery := "SELECT " + policyColumns + " FROM policies WHERE id = $1 FOR UPDATE"
	getArgs := []interface{}{id}
	if clause, arg := primitive.OwnerScope(ctx, 2); clause != "" {
		getQuery += " AND " + clause
		getArgs = append(getArgs, arg)
	}
	cur, err := scanPolicy(tx.QueryRowContext(ctx, getQuery, getArgs...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, primitive.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	newVersion := cur.Version + 1
	var agentID interface{}
	if p.AgentID != "" {
		agentID = p.AgentID
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE policies SET scope = $1, agent_id = $2, rules = $3, enabled = $4,
		                   version = $5, updated_at = NOW()
		WHERE id = $6`,
		p.Scope, agentID, rulesJSON, p.Enabled, newVersion, id); err != nil {
		return nil, fmt.Errorf("failed to update policy: %w", err)
	}
	if err := auditInsert(tx, ctx, id, change, cur.Version, newVersion, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit policy update: %w", err)
	}
	return &Policy{
		ID: id, OwnerID: cur.OwnerID, Scope: p.Scope, AgentID: p.AgentID,
		Rules: p.Rules, Enabled: p.Enabled, Version: newVersion,
	}, nil
}

// Delete removes a policy. The 'deleted' audit row is written FIRST;
// policy_audit.policy_id is ON DELETE SET NULL, so it survives the
// deletion as the permanent record that the policy existed and who
// removed it.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin policy txn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	getQuery := "SELECT " + policyColumns + " FROM policies WHERE id = $1 FOR UPDATE"
	getArgs := []interface{}{id}
	if clause, arg := primitive.OwnerScope(ctx, 2); clause != "" {
		getQuery += " AND " + clause
		getArgs = append(getArgs, arg)
	}
	cur, err := scanPolicy(tx.QueryRowContext(ctx, getQuery, getArgs...))
	if errors.Is(err, sql.ErrNoRows) {
		return primitive.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := auditInsert(tx, ctx, id, "deleted", cur.Version, 0, nil); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM policies WHERE id = $1`, id); err != nil {
		return fmt.Errorf("failed to delete policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit policy delete: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------
// Proposals (agent drafts, human approval)
// --------------------------------------------------------------------

// Draft is the shape an agent (or operator) may propose as a new
// policy. Validated exactly like a directly-created policy.
type Draft struct {
	Scope   string            `json:"scope"`
	AgentID string            `json:"agent_id,omitempty"`
	Rules   []validation.Rule `json:"rules"`
}

// ParseDraft validates a proposal draft body.
func ParseDraft(raw []byte) (*Draft, error) {
	d := &Draft{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, fmt.Errorf("invalid draft: %w", err)
	}
	if err := validation.ValidatePolicyScope(d.Scope, d.AgentID); err != nil {
		return nil, err
	}
	if err := validation.ValidateRules(d.Rules); err != nil {
		return nil, err
	}
	return d, nil
}

// RecordProposal stores a validated draft as a policy_audit row with
// change='proposal' and policy_id NULL. The row's id IS the proposal
// id (returned to the caller). The draft JSON rides in detail. This is
// how agents "propose" rules: the row is inspectable + auditable but
// creates no policy until a non-restricted key approves it.
func (s *Store) RecordProposal(ctx context.Context, draft *Draft) (string, error) {
	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return "", fmt.Errorf("failed to encode proposal: %w", err)
	}
	id := uuid.New().String()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_audit (id, policy_id, actor, change, prev_version, new_version, detail)
		VALUES ($1, NULL, $2, 'proposal', NULL, NULL, $3)`,
		id, actor(ctx), draftJSON); err != nil {
		return "", fmt.Errorf("failed to record proposal: %w", err)
	}
	return id, nil
}

// ApproveProposal materializes an approved proposal: creates the
// policy (owner = the approver's owner, version 1, enabled) and
// morphs the proposal audit row into an 'approved-proposal' row
// pointing at the new policy — all in one transaction. Only reachable
// with a non-restricted key (the H3.1 restricted-class route check
// blocks restricted keys on this route).
func (s *Store) ApproveProposal(ctx context.Context, proposalID string) (*Policy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin proposal txn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var detailJSON []byte
	err = tx.QueryRowContext(ctx, `
		SELECT detail FROM policy_audit
		WHERE id = $1 AND change = 'proposal' AND policy_id IS NULL
		FOR UPDATE`, proposalID).Scan(&detailJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, primitive.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read proposal: %w", err)
	}
	draft, err := ParseDraft(detailJSON)
	if err != nil {
		// Should not happen (validated at record time); a corrupted
		// draft must not half-apply.
		return nil, fmt.Errorf("proposal %s holds an invalid draft: %w", proposalID, err)
	}

	pr, ok := primitive.PrincipalFrom(ctx)
	if !ok || pr.UserID == "" {
		return nil, errors.New("approving a proposal requires an authenticated owner")
	}
	pid := uuid.New().String()
	var agentID interface{}
	if draft.AgentID != "" {
		agentID = draft.AgentID
	}
	rulesJSON, err := json.Marshal(draft.Rules)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO policies (id, owner_id, scope, agent_id, rules, enabled, version,
		                      created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, TRUE, 1, NOW(), NOW())`,
		pid, pr.UserID, draft.Scope, agentID, rulesJSON); err != nil {
		return nil, fmt.Errorf("failed to create approved policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE policy_audit SET policy_id = $2, change = 'approved-proposal', new_version = 1
		WHERE id = $1`, proposalID, pid); err != nil {
		return nil, fmt.Errorf("failed to mark proposal approved: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit proposal approval: %w", err)
	}
	return &Policy{
		ID: pid, OwnerID: pr.UserID, Scope: draft.Scope, AgentID: draft.AgentID,
		Rules: draft.Rules, Enabled: true, Version: 1,
	}, nil
}

// RecordMemo records a DECIDED verdict for an action in policy_memo:
// the "human said yes/no" half of the ask round-trip (H3.5 part 2).
// The action hash is computed with HashAction — the same function the
// evaluator uses — so the endpoint and the evaluator can't drift; the
// upsert goes through the shared upsertMemo SQL path.
//
// Only allow/deny are recorded (the caller validates): an 'ask' is a
// pending question, not a decision, and the no-matching-policy default
// is not a decision either.
func (s *Store) RecordMemo(ctx context.Context, agentID, tool string, input map[string]any, verdict, ruleID, reason string) error {
	return upsertMemo(ctx, s.db, agentID, ruleID, HashAction(tool, input), verdict, reason)
}

// queryPolicies runs a policies SELECT and decodes the rows.
func (s *Store) queryPolicies(ctx context.Context, query string, args ...interface{}) ([]*Policy, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Policy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
