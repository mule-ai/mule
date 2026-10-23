# Mule Tenancy (Herd H3.1)

Tenancy parity with forge: multi-user mule deployments where each user's
agents, workflows, jobs, skills, and providers are invisible and
inoperable to every other user, with an admin role for fleet operators.

Migration: `internal/database/migrations/0012_tenancy.sql`
(0011 was taken by H1.5 `agents.external_ref`).

## Data model

- **`users`** — `id VARCHAR(255)` PK, `email` unique, `name`,
  `password_hash` (argon2id PHC string), `role` CHECK
  (`'user'` | `'admin'`), timestamps. Mirrors forge's
  `002_users_and_api_keys.sql` conventions in mule's VARCHAR id style.
- **`api_keys`** — `id`, `user_id` (FK, `ON DELETE CASCADE`),
  `key_hash` (SHA-256 hex of the key **body**, unique), `key_prefix`
  (first 12 chars, for identification), `restricted BOOLEAN`,
  `expires_at`, `created_at`.
- **`owner_id VARCHAR(255)`** on `agents`, `workflows`, `jobs`,
  `skills`, `providers`. No FK to `users` (migration backfill stamps
  the well-known default admin id before any users row exists; an FK
  would deadlock the migration). `tools`, `wasm_modules`, settings, and
  memory config stay shared/global by design in this phase.

## Default bootstrap admin

The Go bootstrap (`internal/api/bootstrap.go`, run from `cmd/api`
`main()` right after `InitSchema`) creates the admin on first boot
only; any boot with existing users is a logged no-op.

| Env var | Default | Notes |
|---|---|---|
| `MULE_ADMIN_ID` | `00000000-0000-4000-8000-000000000001` | Fixed documented UUID so the SQL backfill in migration 0012 and the Go bootstrap agree. If you override it, pre-tenancy rows keep the placeholder owner — migrate them explicitly (`UPDATE ... SET owner_id = '<new id>'`). |
| `MULE_ADMIN_EMAIL` | `admin@localhost` | |
| `MULE_ADMIN_PASSWORD` | generated: 24 random bytes hex | **Logged once** at boot in a banner. |
| `MULE_ADMIN_API_KEY` | generated: `sk_mule_<32 random bytes hex>` | **Logged once** at boot when generated. When set by the operator the value is seeded verbatim and NOT re-logged. |

Operator flow:

- **Local dev**: set all three `MULE_ADMIN_*` vars (id can stay at the
  default); the key then comes from your env, never from the log.
- **Fleet**: set them from your secrets store; rotate by inserting a
  new `api_keys` row and deleting the old one (the bootstrap will not
  touch a populated `users` table).
- `MULE_NO_AUTH=1` (below) does not change bootstrap behavior.

## Authentication

Middleware: `internal/api/auth.go` (`AuthMiddleware`), wired in
`cmd/api/server.go` via `router.Use` exactly like the logging
middleware.

- **Protected routes**: every `/api/v1/*` and `/v1/*` request.
- **Unauthenticated**: `/health`, `/ws`, and the frontend catch-all —
  documented intentional exception (health probes and the web UI shell
  must be reachable; `/ws` upgrades cannot carry a Bearer header and
  job-stream frames expose only ids of jobs the socket's viewer could
  already list).
- **Credential**: `Authorization: Bearer sk_mule_<hex>`. The
  `sk_mule_` prefix is stripped exactly once before hashing, so a bare
  key body verifies against the same row (dual-read, mirrors forge).
- **Failures** (JSON, standard mule `ErrorResponse` shape):
  - no/malformed header, unknown key, expired key → **401**
  - restricted key on a write route → **403**
  - lookup error → 500.

## Restricted keys

`api_keys.restricted = TRUE` denies **write** routes on the
agent/workflow/job/policy classes. The route-class check
(`isRestrictedWrite`) is `method + path`:

- methods: `POST`, `PUT`, `PATCH`, `DELETE`
- path prefixes: `/api/v1/agents`, `/api/v1/workflows`, `/api/v1/jobs`,
  `/api/v1/policies` (the last reserved for a future policies surface;
  exact match or `prefix + "/"`)

A restricted key may still READ any of those classes, write
providers/tools/skills/wasm-modules, and EXECUTE via the OpenAI
surface (`/v1/chat/completions`) — the restricted classes are the
mule-native CRUD APIs only.

**Admin does not bypass restricted.** `restricted` is a property of
the KEY, not the user; admin + restricted still gets 403 on those
routes. Deliberate, mirrors forge.

## Owner scoping

The middleware attaches two things to the request context:

- `api.AuthUser` (id, email, role, restricted, noAuth) —
  `internal/api/auth.go`
- `primitive.Principal{UserID, Admin}` — `internal/primitive/ownership.go`

Scoping rules, applied in `internal/primitive/store_pg.go`,
`internal/manager/{skill,workflow,agent,provider}.go`,
`pkg/job/store_pg.go`, and the job/agent/workflow-step handlers in
`cmd/api/handlers.go`:

- **no principal in context** (engine background workers, bootstrap,
  unit tests): legacy behavior — no filter, no `owner_id` written.
  The engine deliberately runs ownerless: a submitted job is trusted to
  run the workflow it was created against.
- **admin principal**: no read filter (sees all owners); writes stamp
  `owner_id = <admin user id>`.
- **non-admin principal**: every list/get on the five tables appends
  `AND owner_id = $n`; update/delete append the same; inserts stamp
  `owner_id`. Workflow steps are scoped through their workflow
  (`workflow_id IN (SELECT id FROM workflows WHERE owner_id = $n)`);
  agent tool/skill junctions through their agent.
- **Jobs**: `jobs.owner_id` is set at submission from the caller's
  principal; `ListJobs` takes `OwnerID`/`Admin` options;
  get/cancel/steps-by-id handlers 404 a job that exists under another
  user. Ownerless (NULL) jobs are admin-only.
- **404, not 403**: foreign objects are reported with mule's existing
  not-found semantics — ownership leaks nothing.

## MULE_NO_AUTH dev mode

`MULE_NO_AUTH=1` turns the middleware into a pass-through that attaches
a **synthetic admin** principal (`DefaultAdminID`, role admin) to every
protected request. Consequences:

- existing local/ranch integrations (`mule_url` pointing at a local
  daemon with no key) keep working unchanged;
- created rows are owned by `00000000-0000-4000-8000-000000000001`,
  so switching auth on later does not strand that data — it is
  reachable by the default admin key;
- the bootstrap still runs (env-independent) and the synthetic admin's
  id equals the default admin id, so scoping is coherent;
- the env is read per request (cheap; keeps the flag testable).

Default (env unset/anything-but-`1`): **auth required** on all
protected routes.

## What is NOT covered in this phase

- No user registration / login / key-minting HTTP endpoints (operators
  manage users/keys in the DB; key issuance flow is the bootstrap +
  direct DB rows).
- No tenancy on `tools`, `wasm_modules`, settings, memory config,
  artifacts, or the `/ws` job stream.
- No audit log (forge has one; mule does not yet).
