# H4.4 Reflection Workflow — "the dogfood"

A **real mule workflow** that turns an agent's recent *episodes* into
*reviewed belief diffs*. This is the canonical learning loop of the
Herd initiative: forge hosts the memory, mule scripts the behavior,
ranch surfaces the approval.

## The loop (end to end)

```
 ┌────────────── forge (H4.2) ──────────────────────────────────┐
 │ every finished turn  ->  deterministic extractor + redaction │
 │  -> episodes row (summary, source seq_range, embedding)      │
 └──────────────────────────────┬───────────────────────────────┘
                                │
 ┌────────────── mule wake (H3.3) ──────────────────────────────┐
 │  cron  "reflection-nightly"        0 3 * * *  (UTC, nightly) │
 │  event "reflection-episodes-ready" manual fire (see below)   │
 │  fire -> SubmitJob on the workflow "agent-reflection"        │
 └──────────────────────────────┬───────────────────────────────┘
                                │
 ┌────────────── mule job: agent-reflection ────────────────────┐
 │ step 1  wasm  pull     GET /agents/:id/memory/search (x N    │
 │                      recall queries, k=50 each) +           │
 │                      GET /agents/:id/memory/beliefs?status=  │
 │                      active  ->  {episodes, beliefs, creds}  │
 │ step 2  agent step     cheap model (pi-based mule agent)     │
 │                      -> STRICT JSON belief-diff array        │
 │ step 3  wasm  check    deterministic gate against the LIVE   │
 │                      pending+active beliefs (re-fetched):   │
 │                      duplicate / contradiction / decay      │
 │ step 4  wasm  submit   POST /agents/:id/memory/beliefs/      │
 │                      proposals  ->  per-belt accepted/      │
 │                      rejected results = job output_data     │
 └──────────────────────────────┬───────────────────────────────┘
                                │
 ┌────────────── forge governance (H4.4 part 1, DONE) ─────────┐
 │ each accepted proposal -> beliefs status='pending' +        │
 │ memory_review approval card on the agent's active           │
 │ conversation (ranch TUI/web/mobile: "I've learned: … —      │
 │ Keep / Forget / Edit")                                       │
 └──────────────────────────────┬───────────────────────────────┘
                                │
 Keep -> active (version bump, audit) -> next turn's
 memory:beliefs prompt section (H4.3)  — the loop closes.
 Forget -> forgotten.  Edit -> new content + active + re-embed.
```

Belief changes are reviewable forever: `GET
/agents/:id/memory/beliefs?status=` + `belief_audit` rows (who/what
proposed/changed, version chain).

## The four steps

| # | type   | what it is | failure mode |
|---|--------|------------|--------------|
| 1 | wasm `pull` | forge recall-based episode pull + active beliefs | no forge / no agent (404) / embedding endpoint down (503) → **failed step** (a run that can't read memory must not silently reflect on nothing) |
| 2 | agent | a mule agent (cheap model, no tools) with the reflection system prompt; proposes `[{op: add\|strengthen\|weaken\|forget\|supersede, belief, confidence, rationale, source_episodes}]`, max 10, STRICT JSON | model output is parsed by step 3 (fences/prose tolerated; a bracketed-but-undecodable array **fails** the step rather than reflect nothing) |
| 3 | wasm `check` | the deterministic gate (below) | gate logic lives in `internal/reflection/gate` and is host-unit-tested; this module is a thin HTTP/JSON shell |
| 4 | wasm `submit` | POSTs the gated diffs to forge | per-belt `accepted/rejected_reason` surfaced in the job output; an empty gated set is a **successful** no-op run |

### Why a plain agent step for step 2 (not `forge_conversation`)

The diff proposal is a **stateless, one-shot, cheap-model** inference
over a bounded input (≤ 50 episodes + active beliefs). A mule agent
step is exactly that: `pi --mode rpc` inside mule, model chosen in the
agent config, no forge conversation, no turn-waiting, no
`[mule job …]` provenance dance. The H3.2 `forge_conversation` step is
for talking to a long-lived *forge* agent conversation — using it
here would park the reflection reasoning inside the very agent being
reflected on (self-hosting the loop in the subject's own transcript),
which the plan explicitly avoids: the reflection loop is **mule**
behavior, and only the *governance* (proposals → cards) is forge's.

### The deterministic gate (step 3) — v1 heuristics, honestly

All in `internal/reflection/gate` (host-tested; no wasm involved):

- **Duplicate** — normalized-string equality
  (case/whitespace-insensitive) against **pending + active** beliefs,
  plus batch-local dedupe. Exact duplicates are `needs_review` AND
  marked: step 4 does NOT resubmit them (forge's own near-dup gate
  would reject them anyway; the actionable bit is the *reason* in the
  job output).
- **Contradiction** — v1 opposite-signal detection, **string-based**:
  a negation-marker set (`never`, `don't`, `do not`, `stop(s)`,
  `avoid(s)`, `no longer`, `refuse(s)`, `without`, …) present in
  exactly one of the pair, AND content-token overlap (Jaccard over
  stopword-filtered lowercase tokens) ≥ 0.4. Both-negative pairs
  ("never X" vs "avoid X") agree in polarity and are NOT flagged;
  marker-free semantic opposition is out of reach here because the
  forge memory API does not return embeddings — at submit time
  forge's cosine near-dup gate (0.92) is the backstop, and a
  genuinely contradictory *add* still lands as a **pending** belief a
  human sees, never active. When it fires: the diff is routed to
  needs_review, never auto-applied.
- **Routing** — ops the proposals endpoint cannot express in v1
  (`strengthen`/`weaken`/`forget`/`supersede` — it only creates
  pending rows) are always needs_review.
- **Decay** — active beliefs decay as
  `confidence · exp(−λ · age_days)`; λ default **0.02/day**
  (configurable via the check module's `lambda_per_day`; half-life
  ~34.7 days). Report-only: a "weaken" of an existing belief would be
  rejected as a duplicate, so the decay report lands in the job
  output and the human applies it on the card.

### What step 4 submits

- `safe` adds — as-is.
- `needs_review` (non-exact-duplicate) — **also submitted**, with
  their gate reason prefixed to the rationale as
  `[needs human review] <reason>` — they become pending beliefs a
  human sees on a card and can Keep/Forget (forget/supersede is then
  applied by hand via the keep/forget doors). This is the v1
  governance bridge for the add-only endpoint.
- exact duplicates — skipped, listed in `skipped`.
- decayed beliefs — job output only (see above).

## Credentials flow (v1)

`forge_url` / `forge_api_key` / `agent_id` live in the **module
config** of the pull/check/submit wasm modules (seed.sh sets them)
and, for transparency, ride in pull's output. This mirrors the H3.2
`forge_conversation` stance: inlined in the primitive, with a
**secrets table + `api_key_ref`** as the documented upgrade path.
Known v1 exposure (documented, not hidden): pull's output — which
contains the key — is passed to the step-2 model as its prompt. The
secrets-table upgrade removes it from the LLM's input entirely.

The engine also injects `mule_time` (host wall clock, RFC3339) into
every wasm step's input: WASI preview1 has no clock, so without it
`time.Now()` in a module is pinned to the WASI epoch
(2022-01-01) and the decay pass would be dead. Module config / job
input may override it (tests pin it).

## Layout

```
examples/reflection/
├── pull/main.go      step 1   (//go:build ignore; built to dist/pull.wasm)
├── check/main.go     step 3
├── submit/main.go    step 4
├── stub/main.go      step-2 stand-in (fixed agent-style text; e2e test + no-LLM runs)
├── Makefile          builds all four: GOOS=wasip1 GOARCH=wasm go build
├── dist/             .wasm artifacts (gitignored; build on demand)
├── reflection.json   the seed manifest, annotated (the exact API payloads)
├── seed.sh           idempotent seeder (agent + 4 modules + workflow + 2 wakes)
└── README.md         this file
internal/reflection/
├── gate/             the deterministic gate + host unit tests
├── reflmem/          shared JSON shapes + agent-output parser + tests
└── wasmhttp/         host-HTTP bridge for the modules (wasip1 import + host stub)
internal/engine/
└── reflection_test.go  engine-level acceptance (below)
```

## Tests

- `internal/reflection/gate` — duplicate normalization,
  negation-contradiction flag (and the both-negative false-positive
  guard), decay math (λ=0.02, 30 days → ×0.5488), safe/needs_review
  split incl. op routing + batch dedupe.
- `internal/reflection/reflmem` — the agent-output parser: bare
  array, ```json fence, brackets inside JSON strings, `[]` = legit
  "nothing", undecodable array = error.
- `internal/engine/reflection_test.go` — the real wasm binaries
  through the real engine against a fake forge (httptest):
  - **full job end to end**: pull → (fixed-output stand-in for the
    agent step — `wasmEchoModule`) → check → submit; asserts exactly
    one proposals POST with the clean add + the prefixed forget, the
    duplicate skipped, and the decayed belief in the job output.
    **The real-model leg (step 2 with an actual LLM) is the manual
    acceptance** — agent steps spawn `pi --mode rpc` and cannot be
    faked in-process.
  - pull: lists episodes + beliefs; 404 agent; 503 embedding-down;
    `since` filter.
  - check: dup + contradiction + forget-op routing + decay.
  - submit: exact POST body shape, per-belt accepted surfaced.
  - stub: emits agent-style text that the parser accepts.

## Seeding & running (manual acceptance)

```bash
export MULE_URL=http://<mule>:8080          # or a MULE_NO_AUTH=1 dev instance
export MULE_API_KEY=sk_mule_...             # optional under MULE_NO_AUTH=1
export FORGE_URL=http://10.10.199.29:8110   # as the MULE host reaches forge
export FORGE_API_KEY=sk_forge_...
export AGENT_ID=<the forge agent to reflect on>
# optional: MULE_MODEL=<cheap model id>   LAMBDA_PER_DAY=0.02

./seed.sh
# manual run:
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"workflow_id":"<wf-id>","input_data":{}}' "$MULE_URL/api/v1/jobs"
```

## Triggers — cron now, episode-counts later

- **Nightly**: wake `reflection-nightly` (cron `0 3 * * *` UTC) →
  submits a job on the workflow. The cron lane fires every day; a day
  with nothing new yields a successful no-op run (the agent replies
  `[]`).
- **"N new episodes"**: v1 is a **manual event fire** —
  `POST /api/v1/wakes/fire {kind:"event", source:"episodes_ready",
  payload:{new_episodes: 20}}` matches the `reflection-episodes-ready`
  wake. **Automatic episode-count detection is a documented
  follow-up**: it is exactly the H4.5 memory trigger (forge watches
  the episode count / a belief `watch` predicate and fires the same
  ingress), which is why the wake is pre-created with that event type
  instead of bolting a poller onto this workflow.

## Limitations (v1, all documented on purpose)

1. **Recall-based pull**: forge has no "list recent episodes"
   endpoint; pull uses N recall queries × k=50 over
   `/memory/search`. A night with an unrecallable episode misses it.
   The H4.5 trigger + a cheap episode-list endpoint close this.
2. **String-based contradiction gate** (above) — conservative by
   construction; the forge cosine gate + the pending-first lifecycle
   are the safety net.
3. **Add-only proposals endpoint** — strengthen/weaken/forget/
   supersede ride as needs_review pending beliefs applied by hand.
4. **Credentials in module config + visible to the step-2 model**
   (above) — secrets table is the upgrade.
5. **No `since` bookkeeping**: pull doesn't remember when the last
   reflection ran (job input may pass `since`); nightly cadence +
   dedupe gates make repeats cheap, and the H4.5 trigger supplies
   exact counts later.
