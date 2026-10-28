package app

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const (
	// deepPlanTTL bounds a deep plan: a scan that outlives it is reported as
	// expired instead of being presented as a total.
	deepPlanTTL = time.Hour
	// defaultDailyCopyLimit is the server-side copy allowance per account per
	// day that Google documents (750 GB). It only drives a warning.
	defaultDailyCopyLimit = int64(750) * 1000 * 1000 * 1000
	// deepSaveEvery throttles progress writes to the plan while a scan runs.
	deepSaveEvery = 100 * time.Millisecond
	nativePrefix  = "application/vnd.google-apps."
)

func (s *Service) deepTTL() time.Duration {
	if s.DeepPlanTTL > 0 {
		return s.DeepPlanTTL
	}
	return deepPlanTTL
}

func (s *Service) dailyLimit() int64 {
	if s.DailyCopyLimit > 0 {
		return s.DailyCopyLimit
	}
	return defaultDailyCopyLimit
}

func (s *Service) baseCtx() context.Context {
	if s.BaseCtx != nil {
		return s.BaseCtx
	}
	return context.Background()
}

// startDeep runs the read-only deep scan of a plan in the background. The
// request that asked for it has already returned; clients poll the plan.
func (s *Service) startDeep(prov drive.Provider, plan state.Plan) {
	ctx, cancel := context.WithDeadline(s.baseCtx(), plan.ExpiresAt)
	s.deepMu.Lock()
	if s.deepCancel == nil {
		s.deepCancel = map[string]context.CancelFunc{}
	}
	s.deepCancel[plan.ID] = cancel
	s.deepMu.Unlock()
	go func() {
		defer func() {
			s.deepMu.Lock()
			delete(s.deepCancel, plan.ID)
			s.deepMu.Unlock()
			cancel()
		}()
		s.runDeep(ctx, prov, plan)
	}()
}

// runDeep walks the source tree page by page, keeping only the folder ids it
// still has to visit. It never writes to Drive; every call is a list or a
// metadata read the copy would make anyway.
func (s *Service) runDeep(ctx context.Context, prov drive.Provider, plan state.Plan) {
	d := state.DeepPreflight{State: "running", EstimateBasis: "unknown", Warnings: []string{}}
	var last time.Time
	save := func(force bool) {
		if !force && time.Since(last) < deepSaveEvery {
			return
		}
		last = time.Now()
		d.UpdatedAt = s.now().UTC()
		_ = s.Store.SavePlanDeep(context.WithoutCancel(ctx), plan.ID, d)
	}
	finish := func(state string) {
		d.State = state
		d.Complete = state == "complete"
		d.UpdatedAt = s.now().UTC()
		_ = s.Store.SavePlanDeep(context.WithoutCancel(ctx), plan.ID, d)
	}

	if plan.SourceKind == state.KindFile {
		d.Files = 1
		if strings.HasPrefix(plan.SourceMime, nativePrefix) {
			d.Native = 1
		} else if plan.SourceSize != nil {
			d.Bytes = *plan.SourceSize
		}
	} else {
		type node struct{ id, key string }
		queue := []node{{plan.SourceID, plan.ResourceKey}}
		for len(queue) > 0 {
			if ctx.Err() != nil {
				break
			}
			cur := queue[0]
			queue = queue[1:]
			if cur.id != plan.SourceID {
				d.Folders++
			}
			token := ""
			for {
				kids, next, err := prov.ListChildren(ctx, cur.id, cur.key, token)
				if err != nil {
					if ctx.Err() != nil {
						break
					}
					switch drive.KindOf(err) {
					case drive.KindPermission, drive.KindNotFound:
						d.Unreadable++
					default:
						finish("failed")
						d.Error = "listing_failed"
						save(true)
						return
					}
					break
				}
				for _, k := range kids {
					d.Scanned++
					switch {
					case k.IsFolder():
						queue = append(queue, node{k.ID, k.ResourceKey})
					case k.IsShortcut():
						d.Shortcuts++
					default:
						d.Files++
						native := strings.HasPrefix(k.MimeType, nativePrefix)
						if native {
							d.Native++
						}
						if !k.CanCopy {
							d.CannotCopy++
							continue
						}
						if !native && k.Size != nil {
							d.Bytes += *k.Size
						}
					}
				}
				save(false)
				if next == "" || ctx.Err() != nil {
					break
				}
				token = next
			}
		}
	}
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			finish("expired")
		} else {
			finish("cancelled")
		}
		return
	}
	s.deepEstimate(context.WithoutCancel(ctx), prov, plan, &d)
	finish("complete")
}

// deepEstimate fills the time estimate (from measured recent copies) and the
// warnings. Nothing here blocks: a copy that would not fit is still allowed to
// start, the warning tells the user before they admit it.
func (s *Service) deepEstimate(ctx context.Context, prov drive.Provider, plan state.Plan, d *state.DeepPreflight) {
	bps, samples, err := s.Store.RecentThroughput(ctx, plan.Principal, 5)
	if err == nil && samples > 0 && bps > 0 {
		secs := int64(math.Ceil(float64(d.Bytes) / bps))
		d.EstimateSecs, d.EstimateBasis = &secs, "measured"
	} else {
		d.Warnings = append(d.Warnings, "estimate_unknown")
	}
	if qr, ok := prov.(drive.QuotaReader); ok {
		limit, usage, qerr := qr.StorageQuota(ctx)
		if qerr == nil && limit > 0 {
			free := limit - usage
			d.StorageFree = &free
			if d.Bytes > free {
				d.Warnings = append(d.Warnings, "destination_storage_short")
			}
		}
	}
	copied, err := s.Store.CopiedBytes24h(ctx, plan.Principal, s.now())
	if err == nil {
		left := s.dailyLimit() - copied
		d.DailyLeft = &left
		if d.Bytes > left {
			d.Warnings = append(d.Warnings, "may_hit_daily_limit")
		}
	}
}

// Plan returns a plan with its deep scan state. A scan recorded as running
// but with no goroutine behind it was cut short by a restart and is reported
// as interrupted; the stored record is not changed.
func (s *Service) Plan(ctx context.Context, p Principal, id string) (PlanView, error) {
	// Ask whether the scan is alive BEFORE reading its record. A scan saves its
	// final state and only then deregisters, so "not running" followed by a read
	// can only see a final state or a real interruption. The other order could
	// read "running", lose a race with a scan that just finished, and report a
	// healthy scan as interrupted.
	running := s.deepRunning(id)
	plan, err := s.Store.GetPlan(ctx, p.ID, id)
	if err != nil {
		return PlanView{}, err
	}
	if plan.Deep != nil && plan.Deep.State == "running" && !running {
		d := *plan.Deep
		d.State = "interrupted"
		plan.Deep = &d
	}
	view := PlanView{Plan: plan, Warnings: []string{}}
	if plan.Deep != nil {
		view.Preview = Preview{ObservedFiles: plan.Deep.Files, ObservedFolders: plan.Deep.Folders, ScanComplete: plan.Deep.Complete}
	}
	return view, nil
}

// CancelPreflight stops a running deep scan. The scan keeps what it counted
// but is never reported complete. Cancelling a scan that is not running changes
// nothing.
func (s *Service) CancelPreflight(ctx context.Context, p Principal, id string) (PlanView, error) {
	view, err := s.Plan(ctx, p, id)
	if err != nil {
		return PlanView{}, err
	}
	if view.Deep == nil || view.Deep.State != "running" {
		return view, nil
	}
	s.deepMu.Lock()
	cancel := s.deepCancel[id]
	s.deepMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.Plan(ctx, p, id)
}

func (s *Service) deepRunning(id string) bool {
	s.deepMu.Lock()
	defer s.deepMu.Unlock()
	_, ok := s.deepCancel[id]
	return ok
}
