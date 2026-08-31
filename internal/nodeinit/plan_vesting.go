package nodeinit

import (
	"fmt"
	"time"
)

// parseUnixTime parses an RFC3339 timestamp to Unix seconds.
func parseUnixTime(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t.Unix(), nil
}

// validateVestingTimeRange checks that a vesting start_time < end_time when both are set.
// Returns an error if the caller passed inconsistent or impossible dates.
func validateVestingTimeRange(endTime, startTime string) error {
	if endTime == "" && startTime != "" {
		return fmt.Errorf("start_time %q requires non-empty end_time", startTime)
	}
	startUnix := int64(0)
	endUnix := int64(0)

	for _, item := range []struct{ s string; l string }{{startTime, "start"}, {endTime, "end"}} {
		if item.s == "" {
			continue
		}
		v, err := parseUnixTime(item.s)
		if err != nil {
			return err
		}
		switch item.l {
		case "start":
			startUnix = v
		case "end":
			endUnix = v
		}
	}

	if endUnix > 0 && startUnix > 0 {
		if startUnix >= endUnix {
			return fmt.Errorf("invalid vesting range: start_time (%s) must be before end_time (%s)",
				startTime, endTime)
		}
	}

	return nil
}


