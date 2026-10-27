package state

import (
	"context"
	"database/sql"
)

const queueActiveSQL = `SELECT fair_group FROM jobs INDEXED BY jobs_fair WHERE state IN ('running','reconciling','pausing','cancelling')`
const queueCandidatesSQL = `SELECT id,fair_group,queue_priority,queue_order,rowid FROM jobs INDEXED BY jobs_queue WHERE state='queued' ORDER BY queue_priority DESC,queue_order,rowid`

// queueSchedule is the shared admission order, including changing fair shares.
func queueSchedule(ctx context.Context, q queryer, limit int) ([]string, error) {
	active := map[string]int{}
	rows, err := q.QueryContext(ctx, queueActiveSQL)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var group string
		if err = rows.Scan(&group); err != nil {
			rows.Close()
			return nil, err
		}
		active[group]++
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, queueCandidatesSQL)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id, group  string
		priority   int
		order, row int64
	}
	var pending []candidate
	for rows.Next() {
		var j candidate
		if err = rows.Scan(&j.id, &j.group, &j.priority, &j.order, &j.row); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, j)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for len(pending) > 0 && (limit == 0 || len(out) < limit) {
		best := 0
		for i := 1; i < len(pending); i++ {
			a, b := pending[i], pending[best]
			if a.priority > b.priority || (a.priority == b.priority && (active[a.group] < active[b.group] || (active[a.group] == active[b.group] && (a.order < b.order || (a.order == b.order && a.row < b.row))))) {
				best = i
			}
		}
		j := pending[best]
		out = append(out, j.id)
		active[j.group]++
		pending = append(pending[:best], pending[best+1:]...)
	}
	return out, nil
}

func (s *Store) QueuePositions(ctx context.Context) (map[string]int, error) {
	positions := map[string]int{}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	order, err := queueSchedule(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	for i, id := range order {
		positions[id] = i + 1
	}
	return positions, tx.Commit()
}
