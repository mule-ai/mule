#!/usr/bin/env bash
# seed.sh — Herd H4.4 reflection workflow: idempotent seeder.
#
# Creates, against a running mule instance, everything the reflection
# loop needs (per the documented payloads in reflection.json):
#
#   1. the mule agent      "reflection-proposer"  (step 2, cheap model)
#   2. the wasm modules    "reflection-{pull,check,submit,stub}"
#   3. the workflow        "agent-reflection"     (pull -> agent -> check -> submit)
#   4. the wakes           "reflection-nightly"   (cron 0 3 * * * -> workflow)
#                          "reflection-episodes-ready" (event -> workflow)
#
# Idempotency: every resource is looked up BY NAME first; existing ones
# are kept as-is (config changes: delete + re-run, or update by hand).
#
# Required env:
#   MULE_URL        e.g. http://10.10.199.29:8091   (mule API base)
#   FORGE_URL       e.g. http://10.10.199.29:8110   (forge API base, as mule reaches it)
#   FORGE_API_KEY   sk_forge_...                    (a key that owns the agent; v1 creds flow)
#   AGENT_ID        the FORGE agent id to reflect on
#
# Optional env:
#   MULE_API_KEY    sk_mule_...  (omit when the mule runs with MULE_NO_AUTH=1)
#   MULE_MODEL      model id for the reflection agent (default: first /v1/models entry)
#   LAMBDA_PER_DAY  decay constant (default 0.02)
#
# Deps: bash, curl, jq, and the Go toolchain (for the first wasm build).

set -euo pipefail
cd "$(dirname "$0")"

# ---------------------------------------------------------------- env
: "${MULE_URL:?MULE_URL is required (mule API base, no trailing slash)}"
: "${FORGE_URL:?FORGE_URL is required (forge API base as MULE reaches it)}"
: "${FORGE_API_KEY:?FORGE_API_KEY is required (sk_forge_...)}"
: "${AGENT_ID:?AGENT_ID is required (the forge agent to reflect on)}"
MULE_URL="${MULE_URL%/}"
FORGE_URL="${FORGE_URL%/}"
LAMBDA_PER_DAY="${LAMBDA_PER_DAY:-0.02}"
AG_NAME="${MULE_AGENT_NAME:-reflection-proposer}"
WF_NAME="${MULE_WORKFLOW_NAME:-agent-reflection}"
WF_NAME="${WF_NAME}"

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

H=()
if [[ -n "${MULE_API_KEY:-}" ]]; then
  H=(-H "Authorization: Bearer ${MULE_API_KEY}")
fi

api() { # api METHOD PATH [JSON_BODY] -> stdout (body)
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "${H[@]}" -H 'Content-Type: application/json' -d "$body" "${MULE_URL}${path}"
  else
    curl -fsS -X "$method" "${H[@]}" "${MULE_URL}${path}"
  fi
}

# ------------------------------------------------------------ wasm build
mkdir -p dist
for m in pull check submit stub; do
  if [[ ! -s "dist/${m}.wasm" ]]; then
    echo "building dist/${m}.wasm ..."
    GOOS=wasip1 GOARCH=wasm go build -o "dist/${m}.wasm" "${m}/main.go"
  fi
done

# ------------------------------------------------------------ provider + model
PROVIDER_ID="$(api GET /api/v1/providers | jq -r '.[0].id // empty')"
if [[ -z "$PROVIDER_ID" ]]; then
  echo "no providers on this mule — create one first (the reflection agent needs a provider)" >&2
  exit 1
fi
MODEL_ID="${MULE_MODEL:-$(api GET /v1/models | jq -r '.[0].id // .[0].model // empty')}"
if [[ -z "$MODEL_ID" ]]; then
  echo "could not find a model id (set MULE_MODEL)" >&2
  exit 1
fi

# ------------------------------------------------------------ agent
AGENT_SYSTEM_PROMPT='You are a memory reflection loop. You receive one JSON object; under its "prompt" key are the recent EPISODES and current BELIEFS of the agent you reflect on (plus transport fields you must ignore and never repeat). Review the episodes and propose belief diffs they support. Reply with a STRICT JSON array (optionally inside a ```json fence) of AT MOST 10 objects of the exact shape {"op": "add"|"strengthen"|"weaken"|"forget"|"supersede", "belief": "one sentence the agent can act on", "confidence": number between 0 and 1, "rationale": "why, citing the episodes", "source_episodes": ["episode uuid from the input"]}. Rules: only propose what the episodes support; use "add" for new durable facts or preferences; cite only episode ids that appear in the input; if nothing is durable, reply with []. No prose outside the JSON array.'

if AGENT_ID_MULE="$(api GET /api/v1/agents | jq -r --arg n "$AG_NAME" '.[] | select(.name == $n) | .id' | head -1)" && [[ -n "$AGENT_ID_MULE" ]]; then
  echo "agent          ${AG_NAME} already exists (${AGENT_ID_MULE}) — keeping it"
else
  AGENT_ID_MULE="$(api POST /api/v1/agents "$(jq -n \
    --arg name "$AG_NAME" \
    --arg desc "H4.4 memory reflection: turns an agent's episodes into reviewed belief diffs (cheap model, no tools)." \
    --arg provider "$PROVIDER_ID" \
    --arg model "$MODEL_ID" \
    --arg prompt "$AGENT_SYSTEM_PROMPT" \
    '{name:$name, description:$desc, provider_id:$provider, model_id:$model, system_prompt:$prompt}')" \
    | jq -r '.id')"
  echo "agent          created ${AG_NAME} (${AGENT_ID_MULE}) model=${MODEL_ID}"
fi

# ------------------------------------------------------------ wasm modules
create_module() { # name description config_json file
  local name="$1" desc="$2" cfg="$3" file="$4"
  local extra=()
  if [[ -n "${MULE_API_KEY:-}" ]]; then
    extra=(-H "Authorization: Bearer ${MULE_API_KEY}")
  fi
  curl -fsS -X POST ${extra[@]+"${extra[@]}"} -F "name=${name}" \
    -F "description=${desc}" -F "config=${cfg}" \
    -F "module_data=@${file}" "${MULE_URL}/api/v1/wasm-modules" | jq -r '.id'
}

module_id() { # name
  # NOTE: GET /api/v1/wasm-modules wraps its list: {"data":[...]}
  api GET /api/v1/wasm-modules | jq -r --arg n "$1" '.data[] | select(.name == $n) | .id' | head -1
}

pull_cfg=$(jq -nc --arg u "$FORGE_URL" --arg k "$FORGE_API_KEY" --arg a "$AGENT_ID" \
  '{forge_url:$u, forge_api_key:$k, agent_id:$a,
    recall_queries:["work session","what happened today","preferences and feedback"], k:50}')
check_cfg=$(jq -nc --arg u "$FORGE_URL" --arg k "$FORGE_API_KEY" --arg a "$AGENT_ID" --arg l "$LAMBDA_PER_DAY" \
  '{forge_url:$u, forge_api_key:$k, agent_id:$a, lambda_per_day:($l|tonumber)}')
submit_cfg=$(jq -nc --arg u "$FORGE_URL" --arg k "$FORGE_API_KEY" --arg a "$AGENT_ID" \
  '{forge_url:$u, forge_api_key:$k, agent_id:$a, actor:"reflection"}')

declare -A MOD_IDS=()
for spec in \
  "reflection-pull:reflection-pull-wasm:pull:dist/pull.wasm:${pull_cfg}" \
  "reflection-check:reflection-check-wasm:check:dist/check.wasm:${check_cfg}" \
  "reflection-submit:reflection-submit-wasm:submit:dist/submit.wasm:${submit_cfg}" \
  "reflection-stub:reflection-stub-wasm:stub:dist/stub.wasm:{}"
do
  IFS=':' read -r mname mdesc mkey mfile mcfg <<<"$spec"
  existing="$(module_id "$mname")"
  if [[ -n "$existing" ]]; then
    echo "wasm module  ${mname} already exists (${existing}) — keeping it"
  else
    existing="$(create_module "$mname" "$mdesc" "$mcfg" "$mfile")"
    echo "wasm module  created ${mname} (${existing})"
  fi
  MOD_IDS[$mname]="$existing"
done

# ------------------------------------------------------------ workflow
if WF_ID="$(api GET /api/v1/workflows | jq -r --arg n "$WF_NAME" '.[] | select(.name == $n) | .id' | head -1)" && [[ -n "$WF_ID" ]]; then
  echo "workflow       ${WF_NAME} already exists (${WF_ID}) — keeping it"
else
  WF_ID="$(api POST /api/v1/workflows "$(jq -n \
    --arg n "$WF_NAME" \
    --arg d "H4.4: turns the agent's episodes into reviewed belief diffs. pull -> agent -> check -> submit." \
    '{name:$n, description:$d, is_async:true}')" | jq -r '.id')"
  echo "workflow       created ${WF_NAME} (${WF_ID})"
fi

STEP_COUNT="$(api GET "/api/v1/workflows/${WF_ID}/steps" | jq 'length')"
if [[ "$STEP_COUNT" -eq 0 ]]; then
  api POST "/api/v1/workflows/${WF_ID}/steps" \
    "$(jq -n --arg w "$WF_ID" --arg m "${MOD_IDS[reflection-pull]}" '{workflow_id:$w, step_order:1, type:"wasm_module", wasm_module_id:$m}')";
  api POST "/api/v1/workflows/${WF_ID}/steps" \
    "$(jq -n --arg w "$WF_ID" --arg a "$AGENT_ID_MULE" '{workflow_id:$w, step_order:2, type:"agent", agent_id:$a}')";
  api POST "/api/v1/workflows/${WF_ID}/steps" \
    "$(jq -n --arg w "$WF_ID" --arg m "${MOD_IDS[reflection-check]}" '{workflow_id:$w, step_order:3, type:"wasm_module", wasm_module_id:$m}')";
  api POST "/api/v1/workflows/${WF_ID}/steps" \
    "$(jq -n --arg w "$WF_ID" --arg m "${MOD_IDS[reflection-submit]}" '{workflow_id:$w, step_order:4, type:"wasm_module", wasm_module_id:$m}')";
  echo "steps          4 steps created"
else
  echo "steps          workflow already has ${STEP_COUNT} step(s) — keeping them"
fi

# ------------------------------------------------------------ wakes
wake_exists() { # name
  # NOTE: GET /api/v1/wakes wraps its list: {"wakes":[...]}
  api GET /api/v1/wakes | jq -r --arg n "$1" '.wakes[] | select(.name == $n) | .id' | head -1
}

if [[ -n "$(wake_exists reflection-nightly)" ]]; then
  echo "wake           reflection-nightly already exists — keeping it"
else
  api POST /api/v1/wakes "$(jq -n --arg wf "$WF_ID" \
    '{name:"reflection-nightly", kind:"cron", spec:{expr:"0 3 * * *"},
      target:{workflow_id:$wf},
      prompt_template:"Nightly reflection over the agent recent episodes.",
      enabled:true}')" >/dev/null
  echo "wake           created reflection-nightly (cron 0 3 * * *)"
fi

if [[ -n "$(wake_exists reflection-episodes-ready)" ]]; then
  echo "wake           reflection-episodes-ready already exists — keeping it"
else
  api POST /api/v1/wakes "$(jq -n --arg wf "$WF_ID" \
    '{name:"reflection-episodes-ready", kind:"event", spec:{type:"episodes_ready"},
      target:{workflow_id:$wf},
      prompt_template:"Episode burst detected: {{.payload}}",
      enabled:true}')" >/dev/null
  echo "wake           created reflection-episodes-ready (event episodes_ready)"
fi

# ------------------------------------------------------------ done
cat <<EOF

Seeded the H4.4 reflection workflow. Manual run:

  curl -fsS -X POST ${H[*]-} -H 'Content-Type: application/json' \\
    -d '{"workflow_id":"${WF_ID}","input_data":{}}' \\
    "${MULE_URL}/api/v1/jobs"

Manual "N new episodes" event fire (v1 trigger; see README.md):

  curl -fsS -X POST -H 'Content-Type: application/json' \\
    -d '{"kind":"event","source":"episodes_ready","payload":{"new_episodes":20}}' \\
    "${MULE_URL}/api/v1/wakes/fire"

Then watch: GET /api/v1/jobs (output_data = submit step's result),
and the agent's pending beliefs in forge:
  curl -fsS -H 'Authorization: Bearer ${FORGE_API_KEY}' "${FORGE_URL}/agents/${AGENT_ID}/memory/beliefs?status=pending"
EOF
