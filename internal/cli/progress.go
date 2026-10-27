package cli

import "github.com/itsgiahung/502Drive/internal/present"

func (j jobView) progress(samples []present.Sample) string {
	c := j.Counts
	return present.RecentProgress(present.Counts{Completed: c.Completed, Total: c.Total, Bytes: c.Bytes, Failed: c.Failed, Blocked: c.Blocked}, samples, "en")
}
