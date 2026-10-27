package present

import "time"

// Sample is an observation, never a lifetime-average estimate.
type Sample struct {
	At        time.Time
	Completed int
}

func AppendSample(samples []Sample, next Sample) []Sample {
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		if !next.At.After(last.At) {
			return samples
		}
		if next.Completed < last.Completed {
			samples = nil
		}
	}
	samples = append(samples, next)
	first := 0
	for first < len(samples) && next.At.Sub(samples[first].At) > 30*time.Second {
		first++
	}
	return append([]Sample(nil), samples[first:]...)
}

// Estimate withholds ETA on stalls, resets, unknown totals or uneven intervals.
func Estimate(c Counts, samples []Sample) (rate float64, eta time.Duration) {
	if len(samples) < 3 {
		return 0, 0
	}
	start, end := samples[0], samples[len(samples)-1]
	dt := end.At.Sub(start.At).Seconds()
	if dt <= 0 || end.Completed < start.Completed {
		return 0, 0
	}
	rate = float64(end.Completed-start.Completed) / dt
	if rate <= 0 {
		return 0, 0
	}
	stable := true
	for i := 1; i < len(samples); i++ {
		dt := samples[i].At.Sub(samples[i-1].At).Seconds()
		if dt <= 0 {
			return 0, 0
		}
		r := float64(samples[i].Completed-samples[i-1].Completed) / dt
		if r < rate/2 || r > rate*2 {
			stable = false
		}
	}
	if stable && c.Total != nil && *c.Total > c.Completed {
		eta = time.Duration(float64(*c.Total-c.Completed) / rate * float64(time.Second))
	}
	return rate, eta
}

func RecentProgress(c Counts, samples []Sample, lang string) string {
	rate, eta := Estimate(c, samples)
	return Progress(c, rate, eta, lang)
}
