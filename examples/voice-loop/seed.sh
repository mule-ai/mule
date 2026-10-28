#!/usr/bin/env bash
# seed.sh — Herd H6.3 voice-loop workflow: idempotent seeder.
#
# Creates, against a running mule instance, the voice-call workflow
# (single `voice_call` step; per the documented payload in
# voice-loop.json):
#
#   1. the workflow       "voice-call"   (voice_call step)
#
# No mule agents or wasm modules: the voice loop is orchestrated
# in-process by mule's voice package (see the example's README and
# spec.md "Voice loop (H6.3)"). One CALL is one mule job submitted to
# this workflow — surfaces do that with POST /api/v1/jobs:
#
#   curl -fsS -X POST "${MULE_URL}/api/v1/jobs" \
#     -H 'Content-Type: application/json' \
#     -d "{\"workflow_id\":\"$(api GET /api/v1/workflows | jq -r '.[] | select(.name=="voice-call") | .id')\"}"
#
# then attach to ws://mule/voice/{job_id}/in|out.
#
# Required env:
#   MULE_URL        e.g. http://10.10.199.29:8091   (mule API base)
#   FORGE_URL       e.g. http://10.10.199.29:8110   (forge API base, as mule reaches it)
#   FORGE_API_KEY   sk_forge_...                    (a key that owns the agent; v1 creds flow)
#   AGENT_ID        the FORGE agent id to call
#
# Optional env:
#   MULE_API_KEY    sk_mule_...  (omit when the mule runs with MULE_NO_AUTH=1)
#   TTS_VOICE       Kokoro voice (default: af_heart)
#   TTS_FORMAT      out-socket audio format (default: pcm)
#   TTS_RATE        out-socket sample rate (default: 24000)
#
# Deps: bash, curl, jq.

set -euo pipefail
cd "$(dirname "$0")"

# ---------------------------------------------------------------- env
: "${MULE_URL:?MULE_URL is required (mule API base, no trailing slash)}"
: "${FORGE_URL:?FORGE_URL is required (forge API base as MULE reaches it)}"
: "${FORGE_API_KEY:?FORGE_API_KEY is required (sk_forge_...)}"
: "${AGENT_ID:?AGENT_ID is required (the forge agent to call)}"
MULE_URL="${MULE_URL%/}"
FORGE_URL="${FORGE_URL%/}"
TTS_VOICE="${TTS_VOICE:-af_heart}"
TTS_FORMAT="${TTS_FORMAT:-pcm}"
TTS_RATE="${TTS_RATE:-24000}"
# VAD/barge-in levels. The defaults are tuned for PHONE calls on
# speakerphone: the mic hears the assistant's own voice out of the
# speaker, and 300/600 let that echo barge-in over the answer (2026-10-28
# "no audio" report: every turn flushed at playback start, out_dropped 0
# but barge_ins == turns). Loud human interruption still clears 1400.
VAD_THRESHOLD="${VAD_THRESHOLD:-450}"
BARGE_IN_THRESHOLD="${BARGE_IN_THRESHOLD:-1400}"
SILENCE_MS="${SILENCE_MS:-700}"
WF_NAME="${MULE_WORKFLOW_NAME:-voice-call}"

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

# ------------------------------------------------------------ workflow
WF_ID="$(api GET /api/v1/workflows | jq -r --arg n "$WF_NAME" '.[] | select(.name == $n) | .id // empty')"
if [[ -n "$WF_ID" ]]; then
  echo "workflow '$WF_NAME' already exists: $WF_ID (config changes: delete + re-run)"
else
  WF_ID="$(api POST /api/v1/workflows \
    "{\"name\":\"$WF_NAME\",\"description\":\"Herd H6.3 voice loop: one job = one call (VAD, STT, agent steer, TTS, barge-in v1)\",\"is_async\":true}" \
    | jq -r '.id')"
  echo "created workflow '$WF_NAME': $WF_ID"
fi

# ------------------------------------------------------------ step
STEPS="$(api GET "/api/v1/workflows/${WF_ID}/steps")"
if [[ "$(echo "$STEPS" | jq 'length')" -gt 0 ]]; then
  echo "workflow already has steps: $STEPS"
  echo "(voice_call config changes: delete the step + re-run, or update by hand)"
else
  STEP_CONFIG="$(jq -n \
    --arg forge_url "$FORGE_URL" \
    --arg api_key "$FORGE_API_KEY" \
    --arg agent_id "$AGENT_ID" \
    --arg tts_voice "$TTS_VOICE" \
    --arg tts_format "$TTS_FORMAT" \
    --argjson tts_rate "$TTS_RATE" \
    --argjson silence_ms "$SILENCE_MS" \
    --argjson vad_threshold "$VAD_THRESHOLD" \
    --argjson barge_in_threshold "$BARGE_IN_THRESHOLD" \
    '{
      forge_url: $forge_url,
      api_key: $api_key,
      agent_id: $agent_id,
      mode: "steer",
      tts_voice: $tts_voice,
      tts_response_format: $tts_format,
      tts_sample_rate: $tts_rate,
      silence_ms: $silence_ms,
      frame_ms: 30,
      vad_threshold: $vad_threshold,
      barge_in_threshold: $barge_in_threshold,
      turn_timeout_secs: 120
    }')"
  api POST "/api/v1/workflows/${WF_ID}/steps" \
    "{\"type\":\"voice_call\",\"step_order\":1,\"config\":$STEP_CONFIG}" \
    > /dev/null
  echo "added the voice_call step (forge: ${FORGE_URL}, agent: ${AGENT_ID}, tts: ${TTS_VOICE}/${TTS_FORMAT}@${TTS_RATE})"
fi

echo
echo "seeding complete. To start a call:"
echo "  JOB_ID=\$(curl -fsS -X POST \"${MULE_URL}/api/v1/jobs\" \\"
echo "    -H 'Content-Type: application/json' -d '{\"workflow_id\":\"${WF_ID}\"}' | jq -r '.id')"
echo "  # then attach:  ws://${MULE_URL#http://}/voice/\$JOB_ID/in   (PCM in + {\"hangup\":true})"
echo "                 ws://${MULE_URL#http://}/voice/\$JOB_ID/out  (TTS out)"
