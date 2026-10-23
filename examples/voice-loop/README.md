# voice-loop — the Herd H6.3 call

One mule job = one push-to-talk "call" with a forge agent. The job
stays RUNNING for the whole call (mule owns the async boundary) and
completes when the call is hung up; a crash/kill mid-call fails the
job — both surfaces render that as "call dropped" (the forge
conversation is intact: committed turns are history).

Full contract: `spec.md` § "Voice loop (H6.3)". This README covers the
data path, the transport decision, and how to seed/drive the example.

## What's here

- `voice-loop.json` — the documented workflow payload (idempotent
  seeder's source of truth).
- `seed.sh` — creates the `voice-call` workflow + its single
  `voice_call` step against a running mule.

**No wasm modules, on purpose.** The voice loop is a long-lived,
bidirectional audio state machine (30 ms frames, blocking STT/agent/
TTS hops, in-flight barge-in). Mule's wasm steps are one-shot
request/response — making a wasm step block for minutes and ferry
per-frame audio through the import ABI would be untestable theater.
The loop therefore runs in Go (`internal/voice`, orchestrated from a
new `voice_call` engine step type) and the workflow JSON is the
orchestration *document*: it carries the call config (forge creds,
voice, thresholds) and pins the job-to-call mapping. The seam stays
clean: the engine sees only a `VoiceExecutor`/`VoiceCall` interface;
`internal/voice`'s `Hub` implements it. If a future mule gets
persistent channels in the wasm ABI, the VAD/barge-in state machines
(already pure functions) are the first candidates to move.

## Data path (one utterance)

```
phone/host                     mule (job goroutine tree)                 forge
   |  16k mono s16le, 30ms          |                                       |
   |==== WS /voice/{job}/in ======>>| VAD: RMS>=vad_threshold ? speech      |
   |                                |  (silence >= silence_ms=600ms)       |
   |                                |-- WAV-wrap (44B RIFF, 16k mono)     |
   |                                |=== POST /v1/audio/transcriptions ==>|
   |                                |<== {"text": ...} (Parakeet)         |
   |                                |-- "[voice call] <text>"             |
   |                                |=== POST /messages (+SSE turn wait) ==>|
   |                                |<== final assistant text            |
   |                                |-- sentence chunks                  |
   |                                |=== POST /v1/audio/speech (xN) =====>|
   |<== WS /voice/{job}/out (PCM)   |<== audio/pcm chunks (Kokoro, 24k)  |
   |    {"flush":true} on barge-in  | out-queue drained + flush control |
   |==== barge-in: hot in-frames ===| TTS stream stops, new STT buffer   |
   |    {"hangup":true} / close     |  -> step output {status,turns,...}  |
   |                                |  -> job COMPLETED (or FAILED)       |
```

- **Barge-in v2 (H6.4, full-duplex)**: the in-VAD runs in every state
  and is pumped in real time around the blocking STT/agent/TTS hops
  (v1 only drained in-frames between out-chunks). A `barge_in_threshold`
  hit for 3 consecutive frames (~90 ms) stops the TTS, **drains the
  out-queue** (queued chunks are dropped, not sent), and emits a
  `{"flush":true}` text frame on the out socket so the phone drops its
  playback buffer too (stop-to-flush is asserted < 150 ms mule-side in
  the unit tests; the wire + phone tail is the client's budget). The
  new utterance is continuous across the state change (no re-arming
  gap). Speech overlapping an in-flight *turn* (thinking/speaking) is
  submitted as a fire-and-forget **steer** — an early partial at the
  first ~`silence_ms` pause (tagged), the full utterance at endpoint.
  Endpointing is the H6.4 ratio rule: silence ≥ `silence_ms` (default
  500) **and** speech:silence RMS ratio ≥ `endpoint_ratio` (default
  3.0), with a `max_utterance_ms` hard cap (default 15 s).
- **Out-queue backpressure**: if a client is slower than TTS, the
  out-queue (cap 64) drops its OLDEST event (counted as `out_dropped`
  in the step output) — out pressure never pauses in-VAD bookkeeping.
- **Out-socket format**: `tts_response_format`/`tts_sample_rate` from
  the step config (v1: `pcm` @ 24 kHz — Kokoro's default rate). The
  out socket carries no metadata; surfaces read the config.
- **Degraded hops**: an STT/agent/TTS failure is recorded in the job
  output's `dropped[]` and the call keeps listening (TTS down ⇒ the
  call still works as dictation).

## Seed

```bash
MULE_URL=http://…:8091 FORGE_URL=http://…:8110 \
FORGE_API_KEY=sk_forge_… AGENT_ID=… ./seed.sh
# optional: TTS_VOICE=am_adam TTS_FORMAT=pcm TTS_RATE=24000
```

## Start a call + attach

```bash
JOB_ID=$(curl -fsS -X POST "$MULE_URL/api/v1/jobs" \
  -H 'Content-Type: application/json' -d '{"workflow_id":"'$WF_ID'"}' | jq -r .id)
# audio in:  ws://mule/voice/$JOB_ID/in    (binary PCM frames ~30ms;
#            text {"hangup":true} to hang up, {"text": …} = typed utterance)
# audio out: ws://mule/voice/$JOB_ID/out   (binary TTS chunks;
#            text {"assistant": …} per turn; {"flush":true} = barge-in
#            → drop your playback buffer; {"done":true} at end)
```

Surfaces: `ranch call <agent>` (ranch-2 CLI) and the mobile-native
agent-row 📞 (Kotlin `CallActivity`) do exactly this — job create
through the mule jobs API, then the two sockets, mic/typed text in,
player out.

## Kill semantics

`Kill(err)` on the live call (in-process crash/kill) or the job's
context ending (timeout/cancel) fails the job, closes both sockets,
and unregisters the call from the hub (no zombie WS handlers). Unit
covered: `internal/voice` (kill mid-call, WS close paths) +
`internal/engine` (`TestVoiceCallStepFailsWhenCallKilled`: real
engine, job marked FAILED, call forgotten).
