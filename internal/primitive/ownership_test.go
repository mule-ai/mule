package primitive

import (
	"context"
	"testing"
)

func TestPrincipalFromAbsent(t *testing.T) {
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("bare context must not carry a principal")
	}
}

func TestOwnerScopeVariants(t *testing.T) {
	cases := []struct {
		name    string
		pr      Principal
		want    string
		wantArg string
	}{
		{"no principal", Principal{}, "", ""},
		{"admin", Principal{UserID: "admin", Admin: true}, "", ""},
		{"empty user", Principal{UserID: ""}, "", ""},
		{"user", Principal{UserID: "u1", Admin: false}, "owner_id = $5", "u1"},
	}
	for _, c := range cases {
		ctx := context.Background()
		if c.pr.UserID != "" {
			ctx = WithPrincipal(ctx, c.pr)
		}
		got, arg := OwnerScope(ctx, 5)
		if got != c.want || (arg != nil && arg.(string) != c.wantArg) {
			t.Errorf("%s: got (%q, %v), want (%q, %q)", c.name, got, arg, c.want, c.wantArg)
		}
	}
}

func TestWorkflowAndAgentOwnerScope(t *testing.T) {
	admin := WithPrincipal(context.Background(), Principal{UserID: "a", Admin: true})
	if got, _ := WorkflowOwnerScope(admin, 2); got != "" {
		t.Fatalf("admin must not get a workflow scope clause, got %q", got)
	}
	if got, _ := AgentOwnerScope(admin, 2); got != "" {
		t.Fatalf("admin must not get an agent scope clause, got %q", got)
	}

	user := WithPrincipal(context.Background(), Principal{UserID: "u1"})
	wfClause, wfArg := WorkflowOwnerScope(user, 2)
	if wfClause != "workflow_id IN (SELECT id FROM workflows WHERE owner_id = $2)" {
		t.Fatalf("unexpected workflow scope clause: %q", wfClause)
	}
	if wfArg.(string) != "u1" {
		t.Fatalf("unexpected workflow scope arg: %v", wfArg)
	}

	agClause, agArg := AgentOwnerScope(user, 3)
	if agClause != "agent_id IN (SELECT id FROM agents WHERE owner_id = $3)" {
		t.Fatalf("unexpected agent scope clause: %q", agClause)
	}
	if agArg.(string) != "u1" {
		t.Fatalf("unexpected agent scope arg: %v", agArg)
	}
}
