package state

import (
	"context"
	"encoding/json"
)

// SavePlanDeep stores the deep scan result (or progress) on the plan body. A
// plan's deep block is read back with GetPlan like the rest of the plan.
func (s *Store) SavePlanDeep(ctx context.Context, planID string, d DeepPreflight) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE plans SET body=json_set(body,'$.deep',json(?)) WHERE id=?", string(b), planID)
	return err
}

// RecentThroughput is the copy speed in bytes per second over the most recent
// succeeded jobs of a principal (logical bytes divided by each job's own
// created-to-updated span). samples is 0 when nothing has been measured yet.
func (s *Store) RecentThroughput(ctx context.Context, principal string, n int) (bytesPerSec float64, samples int, err error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM jobs WHERE principal=? AND state='succeeded' ORDER BY rowid DESC LIMIT ?", principal, n)
	if err != nil {
		return 0, 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, 0, err
	}
	var bytes int64
	var secs float64
	for _, id := range ids {
		j, e := readJob(ctx, s.db, principal, id)
		if e != nil {
			return 0, 0, e
		}
		span := j.UpdatedAt.Sub(j.CreatedAt).Seconds()
		if span > 0 && j.Counts.Bytes > 0 {
			bytes += j.Counts.Bytes
			secs += span
			samples++
		}
	}
	if samples == 0 || secs == 0 {
		return 0, 0, nil
	}
	return float64(bytes) / secs, samples, nil
}
