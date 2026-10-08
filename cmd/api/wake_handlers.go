package main

// Wake API (Herd H3.3): owner-scoped CRUD over the fleet wake-policy
// engine (internal/wake) plus the external fire ingress.
//
// Auth choice (documented in spec.md "Wakes"): wakes are the
// proactivity primitive — the H3.1 restriction that applies to
// agent/workflow/job/policy writes intentionally does NOT apply here;
// any valid owner key gets full CRUD on its own wakes. The fire
// endpoint (POST /api/v1/wakes/fire) is an ingress, not a mutation:
// it accepts any valid key and matches fleet-wide enabled wakes.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/mule-ai/mule/internal/api"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/wake"
)

// wakeAPI is the request/response shape for wake CRUD.
type wakeAPI struct {
	ID               string                 `json:"id,omitempty"`
	OwnerID          string                 `json:"owner_id,omitempty"`
	Name             string                 `json:"name"`
	Kind             string                 `json:"kind"`
	Spec             map[string]interface{} `json:"spec"`
	Target           map[string]interface{} `json:"target"`
	PromptTemplate   string                 `json:"prompt_template"`
	Enabled          *bool                  `json:"enabled"`
	RateLimitPerHour *int                   `json:"rate_limit_per_hour,omitempty"`
	LastFiredAt      *time.Time             `json:"last_fired_at,omitempty"`
	NextDueAt        *time.Time             `json:"next_due_at,omitempty"`
	CreatedAt        *time.Time             `json:"created_at,omitempty"`
	UpdatedAt        *time.Time             `json:"updated_at,omitempty"`
}

func toWakeAPI(w *wake.Wake) wakeAPI {
	a := wakeAPI{
		ID: w.ID, OwnerID: w.OwnerID, Name: w.Name, Kind: string(w.Kind),
		Spec: w.Spec, Target: w.Target, PromptTemplate: w.PromptTemplate,
		LastFiredAt: w.LastFiredAt, NextDueAt: w.NextDueAt,
	}
	if w.Enabled {
		t := true
		a.Enabled = &t
	} else {
		f := false
		a.Enabled = &f
	}
	r := w.RateLimitPerHour
	a.RateLimitPerHour = &r
	if !w.CreatedAt.IsZero() {
		c := w.CreatedAt
		a.CreatedAt = &c
	}
	if !w.UpdatedAt.IsZero() {
		u := w.UpdatedAt
		a.UpdatedAt = &u
	}
	return a
}

// listWakesHandler: GET /api/v1/wakes — the owner's wakes.
func (h *apiHandler) listWakesHandler(w http.ResponseWriter, r *http.Request) {
	wakes, err := h.wakeScheduler.Store().List(r.Context())
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list wakes: %w", err), http.StatusInternalServerError)
		return
	}
	out := []wakeAPI{}
	for _, wk := range wakes {
		out = append(out, toWakeAPI(wk))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"wakes": out})
}

// createWakesHandler: POST /api/v1/wakes
//
//	body: {name, kind, spec, target, prompt_template, enabled?,
//	         rate_limit_per_hour?}
//
// Cron expressions must parse in the supported 5-field subset or the
// request is rejected with 400; next_due_at is computed at creation
// for cron wakes.
func (h *apiHandler) createWakesHandler(w http.ResponseWriter, r *http.Request) {
	var req wakeAPI
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	kind, ok := wake.IsKind(req.Kind)
	if !ok {
		api.HandleError(w, fmt.Errorf("unknown wake kind %q", req.Kind), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		api.HandleError(w, fmt.Errorf("name is required"), http.StatusBadRequest)
		return
	}
	if req.PromptTemplate == "" {
		api.HandleError(w, fmt.Errorf("prompt_template is required"), http.StatusBadRequest)
		return
	}
	if err := wake.ValidateSpec(kind, req.Spec); err != nil {
		api.HandleError(w, fmt.Errorf("invalid spec: %w", err), http.StatusBadRequest)
		return
	}
	if err := wake.ValidateTarget(req.Target); err != nil {
		api.HandleError(w, fmt.Errorf("invalid target: %w", err), http.StatusBadRequest)
		return
	}

	wk := &wake.Wake{
		Name:             req.Name,
		Kind:             kind,
		Spec:             req.Spec,
		Target:           req.Target,
		PromptTemplate:   req.PromptTemplate,
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	if req.Enabled != nil {
		wk.Enabled = *req.Enabled
	}
	if req.RateLimitPerHour != nil {
		wk.RateLimitPerHour = *req.RateLimitPerHour
	}
	if kind == wake.KindCron {
		if nd, err := wake.CronNext(req.Spec["expr"].(string), time.Now().UTC()); err != nil {
			api.HandleError(w, fmt.Errorf("invalid cron expr: %w", err), http.StatusBadRequest)
			return
		} else {
			wk.NextDueAt = &nd
		}
	}
	if err := h.wakeScheduler.Store().Create(r.Context(), wk); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create wake: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toWakeAPI(wk))
}

// getWakesHandler: GET /api/v1/wakes/{id}
func (h *apiHandler) getWakesHandler(w http.ResponseWriter, r *http.Request) {
	wk, err := h.wakeScheduler.Store().Get(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("wake not found: %s", mux.Vars(r)["id"]), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get wake: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toWakeAPI(wk))
}

// patchWakesHandler: PATCH /api/v1/wakes/{id}
//
//	body (all optional): {name?, spec?, target?, prompt_template?,
//	                      enabled?, rate_limit_per_hour?}
//
// The kind is immutable after creation; a new spec is validated
// against the wake's existing kind and a cron wake's next_due_at is
// recomputed when its spec changes.
func (h *apiHandler) patchWakesHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	wk, err := h.wakeScheduler.Store().Get(r.Context(), id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("wake not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get wake: %w", err), http.StatusInternalServerError)
		}
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Apply the patch onto the current row.
	if v, present := patch["name"]; present {
		if s, ok := v.(string); !ok || s == "" {
			api.HandleError(w, fmt.Errorf("name must be a non-empty string"), http.StatusBadRequest)
			return
		} else {
			wk.Name = s
		}
	}
	if v, present := patch["spec"]; present {
		spec, ok := v.(map[string]interface{})
		if !ok {
			api.HandleError(w, fmt.Errorf("spec must be an object"), http.StatusBadRequest)
			return
		}
		if err := wake.ValidateSpec(wk.Kind, spec); err != nil {
			api.HandleError(w, fmt.Errorf("invalid spec: %w", err), http.StatusBadRequest)
			return
		}
		wk.Spec = spec
	}
	if v, present := patch["target"]; present {
		target, ok := v.(map[string]interface{})
		if !ok {
			api.HandleError(w, fmt.Errorf("target must be an object"), http.StatusBadRequest)
			return
		}
		if err := wake.ValidateTarget(target); err != nil {
			api.HandleError(w, fmt.Errorf("invalid target: %w", err), http.StatusBadRequest)
			return
		}
		wk.Target = target
	}
	if v, present := patch["prompt_template"]; present {
		if s, ok := v.(string); !ok || s == "" {
			api.HandleError(w, fmt.Errorf("prompt_template must be a non-empty string"), http.StatusBadRequest)
			return
		} else {
			wk.PromptTemplate = s
		}
	}
	if v, present := patch["enabled"]; present {
		b, ok := v.(bool)
		if !ok {
			api.HandleError(w, fmt.Errorf("enabled must be a boolean"), http.StatusBadRequest)
			return
		}
		wk.Enabled = b
	}
	if v, present := patch["rate_limit_per_hour"]; present {
		switch n := v.(type) {
		case float64:
			if n <= 0 || n > float64(int(^uint(0)>>1)) {
				api.HandleError(w, fmt.Errorf("rate_limit_per_hour must be a positive integer"), http.StatusBadRequest)
				return
			}
			wk.RateLimitPerHour = int(n)
		default:
			api.HandleError(w, fmt.Errorf("rate_limit_per_hour must be an integer"), http.StatusBadRequest)
			return
		}
	}

	// Cron wakes recompute next_due_at whenever the spec changes.
	var nextDue *time.Time
	if wk.Kind == wake.KindCron {
		nd, err := wake.CronNext(wk.Spec["expr"].(string), time.Now().UTC())
		if err != nil {
			// Spec was validated above; this should not happen.
			api.HandleError(w, fmt.Errorf("failed to compute next fire time: %w", err), http.StatusInternalServerError)
			return
		}
		nextDue = &nd
	}
	if err := h.wakeScheduler.Store().Update(r.Context(), wk, nextDue); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("wake not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update wake: %w", err), http.StatusInternalServerError)
		}
		return
	}
	refreshed, err := h.wakeScheduler.Store().Get(r.Context(), id)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to reload wake: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toWakeAPI(refreshed))
}

// deleteWakesHandler: DELETE /api/v1/wakes/{id}
func (h *apiHandler) deleteWakesHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.wakeScheduler.Store().Delete(r.Context(), id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("wake not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete wake: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// wakeLogHandler: GET /api/v1/wakes/{id}/log — newest first, limit 100.
func (h *apiHandler) wakeLogHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, err := h.wakeScheduler.Store().Get(r.Context(), id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("wake not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get wake: %w", err), http.StatusInternalServerError)
		}
		return
	}
	entries, err := h.wakeScheduler.Store().Log(r.Context(), id, 100)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list wake log: %w", err), http.StatusInternalServerError)
		return
	}
	logs := []map[string]interface{}{}
	for _, e := range entries {
		m := map[string]interface{}{
			"fired_at": e.FiredAt,
			"payload":  e.Payload,
			"job_id":   e.JobID,
			"ok":       e.OK,
			"error":    e.Error,
		}
		logs = append(logs, m)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"log": logs})
}

// fireWakesHandler: POST /api/v1/wakes/fire — the external ingress
// (webhooks + any external event push). Any valid API key may call it.
//
//	body: {kind: "webhook"|"event", source, event_tag?, payload?}
//
//	kind "webhook": source + optional event_tag match webhook wakes'
//	                   spec {source, tag?};
//	kind "event":   source is the event TYPE; matches event wakes'
//	                   spec {type}.
//
// Response: {"fired": N, "results": [FireResult per matched wake]}
func (h *apiHandler) fireWakesHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind     string                 `json:"kind"`
		Source   string                 `json:"source"`
		EventTag string                 `json:"event_tag"`
		Payload  map[string]interface{} `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	results, err := h.wakeScheduler.FireExternal(r.Context(), req.Kind, req.Source, req.EventTag, req.Payload)
	if err != nil {
		api.HandleError(w, err, http.StatusBadRequest)
		return
	}
	fired := 0
	for _, res := range results {
		if res.OK {
			fired++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"fired":   fired,
		"results": results,
	})
}
