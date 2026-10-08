package wake

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron semantics: a faithful port of the 5-field cron subset the ranch
// daemon trigger scheduler uses (ranch-2 crates/ranch/src/triggers.rs:
// ~L160, "minimal cron"). Fields: minute hour day-of-month month
// day-of-week. Supported field syntax:
//
//   - `*` — any value in the field's range
//   - `*/n` — every n-th value (step over the full range)
//   - `a` — a single value
//   - `a-b` — an inclusive range
//   - `a,b,c` — a list of the above
//
// Ranges: minute 0-59, hour 0-23, day-of-month 1-31, month 1-12,
// day-of-week 0-6 (0 = Sunday; 7 is NOT accepted, unlike some cron
// flavors). All matching is UTC. This deliberately does NOT support
// `?`, `L`, `W`, month/weekday names, or step-on-range `a-b/n`.
//
// CronNext probes minute by minute, bounded to 366 days — the same
// brute-force strategy as the Rust reference (correct for this subset,
// no dependency on a cron library in the static build).

type cronField struct {
	value    string
	min, max uint
}

var cronFields = []cronField{
	{min: 0, max: 59}, // minute
	{min: 0, max: 23}, // hour
	{min: 1, max: 31}, // day of month
	{min: 1, max: 12}, // month
	{min: 0, max: 6},  // day of week (0 = Sunday)
}

// splitCronFields splits an expression on whitespace into exactly 5
// fields.
func splitCronFields(expr string) ([]string, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields (minute hour dom month dow), got %d", len(fields))
	}
	return fields, nil
}

// validateCronField checks one field of the supported subset.
func validateCronField(field string, f cronField) error {
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return fmt.Errorf("empty cron field part in %q", field)
		}
		rangePart, step := part, 1
		if i := strings.IndexByte(part, '/'); i >= 0 {
			rangePart = part[:i]
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n < 1 {
				return fmt.Errorf("invalid cron step in %q", part)
			}
			step = n
		}
		if rangePart == "*" {
			continue
		}
		if i := strings.IndexByte(rangePart, '-'); i >= 0 {
			lo, err1 := strconv.Atoi(rangePart[:i])
			hi, err2 := strconv.Atoi(rangePart[i+1:])
			if err1 != nil || err2 != nil || lo < int(f.min) || hi > int(f.max) || lo > hi {
				return fmt.Errorf("invalid cron range %q", rangePart)
			}
			continue
		}
		v, err := strconv.Atoi(rangePart)
		if err != nil || v < int(f.min) || v > int(f.max) {
			return fmt.Errorf("invalid cron value %q", rangePart)
		}
		_ = step
	}
	return nil
}

// ValidateCronExpr returns nil when expr is a valid 5-field cron in the
// supported subset.
func ValidateCronExpr(expr string) error {
	fields, err := splitCronFields(expr)
	if err != nil {
		return err
	}
	for i, field := range fields {
		if err := validateCronField(field, cronFields[i]); err != nil {
			return fmt.Errorf("cron field %d: %w", i+1, err)
		}
	}
	return nil
}

// cronCiv converts unix seconds to (month, day, hour, minute, dow)
// where dow is 0 = Sunday, mirroring the Rust reference: the unix epoch
// (1970-01-01) was a Thursday (dow 4), so dow = (days/86400 + 4) mod 7.
func cronCiv(secs int64) (mo, d, h, mi uint, dow uint) {
	t := time.Unix(secs, 0).UTC()
	mo, d, h, mi = uint(t.Month()), uint(t.Day()), uint(t.Hour()), uint(t.Minute())
	days := secs / 86400
	if days < 0 {
		days = (days + 6) / 7 // floor division for pre-epoch safety
	}
	dow = uint((days + 4) % 7)
	return
}

// fieldMatches reports whether one cron field matches a value, mirroring
// the Rust field_matches semantics.
func fieldMatches(field string, value uint, f cronField) bool {
	for _, part := range strings.Split(field, ",") {
		step := 1
		rangePart := part
		if i := strings.IndexByte(part, '/'); i >= 0 {
			rangePart = part[:i]
			if n, err := strconv.Atoi(part[i+1:]); err == nil && n >= 1 {
				step = n
			}
		}
		var lo, hi uint
		switch {
		case rangePart == "*":
			lo, hi = f.min, f.max
		default:
			if i := strings.IndexByte(rangePart, '-'); i >= 0 {
				a, err1 := strconv.Atoi(rangePart[:i])
				b, err2 := strconv.Atoi(rangePart[i+1:])
				if err1 != nil || err2 != nil {
					continue
				}
				lo, hi = uint(a), uint(b)
			} else {
				v, err := strconv.Atoi(rangePart)
				if err != nil {
					continue
				}
				lo = uint(v)
				if step > 1 {
					hi = f.max // single value with step: degenerate, match Rust
				} else {
					hi = uint(v)
				}
			}
		}
		if value >= lo && value <= hi && (value-lo)%uint(step) == 0 {
			return true
		}
	}
	return false
}

// CronMatches reports whether a UTC timestamp matches the expression
// (minute resolution).
func CronMatches(expr string, t time.Time) bool {
	fields, err := splitCronFields(expr)
	if err != nil {
		return false
	}
	mo, d, h, mi, dow := cronCiv(t.Unix())
	return fieldMatches(fields[0], mi, cronFields[0]) &&
		fieldMatches(fields[1], h, cronFields[1]) &&
		fieldMatches(fields[2], d, cronFields[2]) &&
		fieldMatches(fields[3], mo, cronFields[3]) &&
		fieldMatches(fields[4], dow, cronFields[4])
}

// CronNext returns the next fire time strictly after `from` for the
// expression, probing minute by minute (bounded: 366 days). This is the
// "strictly after" convention of the Rust reference: a call at 03:00:30
// for "0 3 * * *" returns tomorrow 03:00, not today 03:00.
func CronNext(expr string, from time.Time) (time.Time, error) {
	if err := ValidateCronExpr(expr); err != nil {
		return time.Time{}, err
	}
	from = from.UTC()
	probe := from.Truncate(time.Minute).Add(time.Minute)
	limit := 366 * 24 * 60
	for i := 0; i < limit; i++ {
		if CronMatches(expr, probe) {
			return probe, nil
		}
		probe = probe.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron expression %q has no fire time within 366 days", expr)
}
