package state

import (
	"fmt"
	"sort"
	"strings"
)

func ParseJobStates(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		switch value {
		case JobQueued, JobRunning, JobPausing, JobPaused, JobCancelling, JobReconciling, JobBlocked, JobSucceeded, JobPartial, JobFailed, JobCancelled:
		default:
			return nil, fmt.Errorf("unknown job state %q", value)
		}
		if !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	sort.Strings(out)
	return out, nil
}
