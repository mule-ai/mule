package primitive

import (
	"context"
	"fmt"
)

// Principal identifies the authenticated caller carried in a request
// context (set by the tenancy auth middleware, cmd/api; see
// internal/api/auth.go and docs/TENANCY.md).
//
//   - UserID: the user id (users.id) the operation acts as.
//   - Admin:  true when the caller has role 'admin' (or is the
//     synthetic MULE_NO_AUTH admin) and therefore sees all owners.
//
// Contexts without a Principal are the pre-tenancy path (engine
// background workers, bootstrap/initialization, unit tests): owner
// scoping is a no-op and no owner_id is written, so existing
// non-HTTP code paths keep working unchanged.
type Principal struct {
	UserID string
	Admin  bool
}

type principalCtxKey struct{}

// WithPrincipal attaches p to ctx for downstream store/manager calls.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom extracts the Principal carried by ctx, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}

// principalScope reports whether ctx carries a non-admin principal
// that must be owner-scoped: a principal is present, not an admin,
// and has a non-empty UserID.
func principalScope(ctx context.Context) bool {
	p, ok := PrincipalFrom(ctx)
	return ok && !p.Admin && p.UserID != ""
}

// OwnerScope returns the additional WHERE clause (with the $param
// placeholder already substituted) that scopes a query on a table
// with an owner_id column to the calling principal, plus the argument
// to bind for it. It returns an empty clause (and nil arg) when ctx
// carries no principal or an admin principal — in that case the caller
// must not append the clause and must not append the arg.
func OwnerScope(ctx context.Context, param int) (string, interface{}) {
	if !principalScope(ctx) {
		return "", nil
	}
	p, _ := PrincipalFrom(ctx)
	return fmt.Sprintf("owner_id = $%d", param), p.UserID
}

// WorkflowOwnerScope returns the additional WHERE clause that scopes a
// row owned indirectly through its workflow (workflow_steps has no
// owner_id of its own; the owning workflow's owner_id applies).
func WorkflowOwnerScope(ctx context.Context, param int) (string, interface{}) {
	if !principalScope(ctx) {
		return "", nil
	}
	p, _ := PrincipalFrom(ctx)
	return fmt.Sprintf("workflow_id IN (SELECT id FROM workflows WHERE owner_id = $%d)", param), p.UserID
}

// AgentOwnerScope returns the additional WHERE clause that scopes a
// row owned indirectly through its agent (agent_tools / agent_skills
// junctions and their lookups).
func AgentOwnerScope(ctx context.Context, param int) (string, interface{}) {
	if !principalScope(ctx) {
		return "", nil
	}
	p, _ := PrincipalFrom(ctx)
	return fmt.Sprintf("agent_id IN (SELECT id FROM agents WHERE owner_id = $%d)", param), p.UserID
}
