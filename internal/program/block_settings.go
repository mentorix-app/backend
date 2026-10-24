package program

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	minRounds     = 1
	maxRounds     = 100
	minSeconds    = 1
	minRestSec    = 0
	maxSecondsDay = 86400
)

// blockSettings are the optional format settings of a group block; nil means unset.
type blockSettings struct {
	Rounds      *int
	DurationSec *int
	TimeCapSec  *int
	WorkSec     *int
	RestSec     *int
}

// OptionalInt tells an absent JSON key (keep), null (clear) and an integer (set) apart.
// A key that is absent never reaches UnmarshalJSON, so Present stays false.
type OptionalInt struct {
	Present bool
	Value   *int
}

func (o *OptionalInt) UnmarshalJSON(data []byte) error {
	o.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("expected an integer or null: %w", err)
	}
	o.Value = &v
	return nil
}

// validateBounds checks every integer the request sets, whatever the block type.
func (in BlockPatchInput) validateBounds() error {
	for _, f := range []struct {
		name     string
		v        OptionalInt
		min, max int
	}{
		{"rounds", in.Rounds, minRounds, maxRounds},
		{"duration_sec", in.DurationSec, minSeconds, maxSecondsDay},
		{"time_cap_sec", in.TimeCapSec, minSeconds, maxSecondsDay},
		{"work_sec", in.WorkSec, minSeconds, maxSecondsDay},
		{"rest_sec", in.RestSec, minRestSec, maxSecondsDay},
	} {
		if f.v.Value != nil && (*f.v.Value < f.min || *f.v.Value > f.max) {
			return fmt.Errorf("%w: %s must be between %d and %d", ErrValidation, f.name, f.min, f.max)
		}
	}
	return nil
}

// resolveSettings returns the settings a block of type resulting ends up with.
// An allowed field takes the request value (nil on null) or keeps the current one;
// a field the type does not allow is reset. A non-null value for such a field is refused.
func (in BlockPatchInput) resolveSettings(resulting BlockType, current blockSettings) (blockSettings, error) {
	rule, ok := resulting.rule()
	if !ok {
		return blockSettings{}, fmt.Errorf("%w: unknown block_type %q", ErrValidation, resulting)
	}
	var out blockSettings
	for _, f := range []struct {
		name    string
		req     OptionalInt
		allowed bool
		current *int
		out     **int
	}{
		{"rounds", in.Rounds, rule.rounds, current.Rounds, &out.Rounds},
		{"duration_sec", in.DurationSec, rule.durationSec, current.DurationSec, &out.DurationSec},
		{"time_cap_sec", in.TimeCapSec, rule.timeCapSec, current.TimeCapSec, &out.TimeCapSec},
		{"work_sec", in.WorkSec, rule.workSec, current.WorkSec, &out.WorkSec},
		{"rest_sec", in.RestSec, rule.restSec, current.RestSec, &out.RestSec},
	} {
		switch {
		case !f.allowed:
			if f.req.Value != nil {
				return blockSettings{}, fmt.Errorf("%w: %s is not allowed for block_type %s", ErrValidation, f.name, resulting)
			}
		case f.req.Present:
			*f.out = f.req.Value
		default:
			*f.out = f.current
		}
	}
	return out, nil
}

func intFromInt32(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func int32FromInt(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// ensureSetsAllowed refuses sets on an exercise of a block whose type carries none.
func ensureSetsAllowed(t BlockType, sets *string) error {
	if sets != nil && !t.allowsSets() {
		return fmt.Errorf("%w: sets are not allowed for block_type %s", ErrValidation, t)
	}
	return nil
}
