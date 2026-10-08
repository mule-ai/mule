package wake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/mule-ai/mule/internal/primitive"
)

// Store is the PostgreSQL store for wakes + wake_log (migration 0014,
// Herd H3.3). Owner scoping follows the H3.1 pattern: request contexts
// carrying a non-admin principal are owner-scoped; the scheduler's
// background contexts carry no principal and are not scoped.
type Store struct {
	db *sql.DB
}

// NewStore creates a wake Store over an existing connection pool.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const wakeColumns = `id, owner_id, name, kind, spec, target, prompt_template,
    enabled, rate_limit_per_hour, last_fired_at, next_due_at, created_at, updated_at`

func scanWake(row interface {
	Scan(dest ...interface{}) error
}) (*Wake, error) {
	w := &Wake{}
	var specJSON, targetJSON []byte
	var lastFired, nextDue sql.NullTime
	err := row.Scan(
		&w.ID, &w.OwnerID, &w.Name, &w.Kind, &specJSON, &targetJSON, &w.PromptTemplate,
		&w.Enabled, &w.RateLimitPerHour, &lastFired, &nextDue, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(specJSON) > 0 {
		if err := json.Unmarshal(specJSON, &w.Spec); err != nil {
			return nil, fmt.Errorf("failed to decode wake spec for %s: %w", w.ID, err)
		}
	}
	if len(targetJSON) > 0 {
		if err := json.Unmarshal(targetJSON, &w.Target); err != nil {
			return nil, fmt.Errorf("failed to decode wake target for %s: %w", w.ID, err)
		}
	}
	if lastFired.Valid {
		t := lastFired.Time
		w.LastFiredAt = &t
	}
	if nextDue.Valid {
		t := nextDue.Time
		w.NextDueAt = &t
	}
	return w, nil
}

// Create inserts a new wake. The owner is taken from the ctx principal
// when present (request path), else w.OwnerID must already be set.
func (s *Store) Create(ctx context.Context, w *Wake) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	if w.OwnerID == "" {
		if p, ok := primitive.PrincipalFrom(ctx); ok && p.UserID != "" {
			w.OwnerID = p.UserID
		}
	}
	if w.OwnerID == "" {
		return errors.New("wakes require an owner")
	}
	if w.RateLimitPerHour <= 0 {
		w.RateLimitPerHour = 24
	}
	specJSON, err := json.Marshal(w.Spec)
	if err != nil {
		return fmt.Errorf("failed to encode wake spec: %w", err)
	}
	targetJSON, err := json.Marshal(w.Target)
	if err != nil {
		return fmt.Errorf("failed to encode wake target: %w", err)
	}
	var lastFired, nextDue interface{}
	if w.LastFiredAt != nil {
		lastFired = *w.LastFiredAt
	}
	if w.NextDueAt != nil {
		nextDue = *w.NextDueAt
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO wakes (id, owner_id, name, kind, spec, target, prompt_template,
		                  enabled, rate_limit_per_hour, last_fired_at, next_due_at,
		                  created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())`,
		w.ID, w.OwnerID, w.Name, string(w.Kind), specJSON, targetJSON,
		w.PromptTemplate, w.Enabled, w.RateLimitPerHour, lastFired, nextDue)
	if err != nil {
		return fmt.Errorf("failed to create wake: %w", err)
	}
	return nil
}

// List returns the owner's wakes (owner-scoped for non-admin principals),
// oldest first.
func (s *Store) List(ctx context.Context) ([]*Wake, error) {
	query := `SELECT ` + wakeColumns + ` FROM wakes`
	args := []interface{}{}
	if clause, arg := primitive.OwnerScope(ctx, 1); clause != "" {
		query += " WHERE " + clause
		args = append(args, arg)
	}
	query += " ORDER BY created_at"
	return s.queryWakes(ctx, query, args...)
}

// Get returns one of the owner's wakes by id (ErrNotFound-style
// primitive.ErrNotFound when absent or not owned).
func (s *Store) Get(ctx context.Context, id string) (*Wake, error) {
	query := `SELECT ` + wakeColumns + ` FROM wakes WHERE id = $1`
	args := []interface{}{id}
	if clause, arg := primitive.OwnerScope(ctx, 2); clause != "" {
		query += " AND " + clause
		args = append(args, arg)
	}
	w, err := scanWake(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, primitive.ErrNotFound
	}
	return w, err
}

// Update replaces the mutable fields of an existing wake. The kind is
// immutable after creation (spec/target are validated against it).
// nextDueAt is set when the (possibly new) spec changes the cron
// schedule; pass nil to leave next_due_at untouched.
func (s *Store) Update(ctx context.Context, w *Wake, nextDueAt *time.Time) error {
	specJSON, err := json.Marshal(w.Spec)
	if err != nil {
		return err
	}
	targetJSON, err := json.Marshal(w.Target)
	if err != nil {
		return err
	}
	query := `
		UPDATE wakes SET name = $1, spec = $2, target = $3, prompt_template = $4,
		               enabled = $5, rate_limit_per_hour = $6, updated_at = NOW()`
	args := []interface{}{w.Name, specJSON, targetJSON, w.PromptTemplate,
		w.Enabled, w.RateLimitPerHour}
	if nextDueAt != nil {
		query += `, next_due_at = $7`
		args = append(args, *nextDueAt)
	}
	query += ` WHERE id = $` + strconv.Itoa(len(args)+1)
	args = append(args, w.ID)
	if clause, arg := primitive.OwnerScope(ctx, len(args)+1); clause != "" {
		query += " AND " + clause
		args = append(args, arg)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return primitive.ErrNotFound
	}
	return nil
}

// Delete removes a wake (and cascades its wake_log rows).
func (s *Store) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM wakes WHERE id = $1`
	args := []interface{}{id}
	if clause, arg := primitive.OwnerScope(ctx, 2); clause != "" {
		query += " AND " + clause
		args = append(args, arg)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return primitive.ErrNotFound
	}
	return nil
}

// queryWakes runs a wakes SELECT and decodes the rows.
func (s *Store) queryWakes(ctx context.Context, query string, args ...interface{}) ([]*Wake, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Wake{}
	for rows.Next() {
		w, err := scanWake(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DueCronWakes returns enabled cron wakes whose next_due_at has passed
// (the cron lane's scan input). Not owner-scoped: the scheduler runs in
// a background context and wakes are fleet-level.
func (s *Store) DueCronWakes(ctx context.Context, now time.Time) ([]*Wake, error) {
	return s.queryWakes(ctx, `
		SELECT `+wakeColumns+` FROM wakes
		WHERE enabled = TRUE AND kind = 'cron'
		  AND next_due_at IS NOT NULL AND next_due_at <= $1
		ORDER BY next_due_at ASC
		LIMIT 100`, now)
}

// MatchingWebhookWakes returns enabled webhook wakes matching the given
// source and (optionally) tag. A wake whose spec has NO tag matches any
// tag (or absence); a wake with a tag matches only that exact tag.
func (s *Store) MatchingWebhookWakes(ctx context.Context, source, tag string) ([]*Wake, error) {
	return s.queryWakes(ctx, `
		SELECT `+wakeColumns+` FROM wakes
		WHERE enabled = TRUE AND kind = 'webhook'
		  AND spec->>'source' = $1
		  AND (spec ? 'tag' = FALSE OR spec->>'tag' = $2)
		ORDER BY created_at ASC`, source, tag)
}

// MatchingEventWakes returns enabled wakes matching an event:
// event-kind wakes whose spec.type equals eventType, plus job-kind wakes
// whose workflow_id + status match (workflowID "" disables the job leg
// — used for external event pushes that know no workflow).
func (s *Store) MatchingEventWakes(ctx context.Context, eventType, workflowID, status string) ([]*Wake, error) {
	return s.queryWakes(ctx, `
		SELECT `+wakeColumns+` FROM wakes
		WHERE enabled = TRUE AND (
			(kind = 'event' AND spec->>'type' = $1)
			OR ($2 <> '' AND kind = 'job' AND spec->>'workflow_id' = $2 AND spec->>'status' = $3)
		)
		ORDER BY created_at ASC`, eventType, workflowID, status)
}

// CountRecentFires counts this wake's wake_log rows in the trailing
// hour — the rate_limit_per_hour guardrail query.
func (s *Store) CountRecentFires(ctx context.Context, wakeID string, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM wake_log
		WHERE wake_id = $1 AND fired_at > $2`, wakeID, now.Add(-time.Hour)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count recent fires for wake %s: %w", wakeID, err)
	}
	return n, nil
}

// RecordFire writes one wake_log row and, when nextDue is non-nil
// (cron wakes), advances last_fired_at/next_due_at in the SAME
// transaction, so a fire cannot be counted twice by the scan lane.
func (s *Store) RecordFire(ctx context.Context, wakeID string, payload map[string]interface{}, jobID *string, ok bool, errMsg *string, nextDue *time.Time) error {
	var payloadJSON []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode fire payload: %w", err)
		}
		payloadJSON = b
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin wake_log txn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wake_log (wake_id, fired_at, payload, job_id, ok, error)
		VALUES ($1, NOW(), $2, $3, $4, $5)`,
		wakeID, payloadJSON, jobID, ok, errMsg); err != nil {
		return fmt.Errorf("failed to write wake_log row: %w", err)
	}
	if nextDue != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE wakes SET last_fired_at = NOW(), next_due_at = $2, updated_at = NOW()
			WHERE id = $1`, wakeID, *nextDue); err != nil {
			return fmt.Errorf("failed to advance wake schedule: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit wake_log txn: %w", err)
	}
	return nil
}

// Log returns up to limit wake_log rows for a wake, newest first.
// (Owner scoping happens on the wake itself — the handler 404s a wake
// that is not the caller's before reaching here.)
func (s *Store) Log(ctx context.Context, wakeID string, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT wake_id, fired_at, payload, job_id, ok, error
		FROM wake_log WHERE wake_id = $1
		ORDER BY fired_at DESC, CTID DESC
		LIMIT $2`, wakeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		e := LogEntry{}
		var payloadJSON []byte
		if err := rows.Scan(&e.WakeID, &e.FiredAt, &payloadJSON, &e.JobID, &e.OK, &e.Error); err != nil {
			return nil, err
		}
		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &e.Payload); err != nil {
				e.Payload = nil
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
