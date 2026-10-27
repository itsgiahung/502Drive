package engine

import (
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Quota handling (Q1). Daily Drive quotas reset at midnight Pacific; a full
// storage account needs a human, but the job still re-checks once a day instead
// of failing permanently. Neither case spends an item attempt.
const (
	storageQuotaRetry = 24 * time.Hour
	unknownQuotaRetry = time.Hour
)

// nextDailyQuotaReset returns the next midnight in America/Los_Angeles. Google
// documents the daily quota reset in Pacific time, including DST.
func nextDailyQuotaReset(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}
	t := now.In(loc)
	next := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	return next.UTC()
}

// quotaResume decides when a quota-blocked job may try again.
func quotaResume(err error, now time.Time) time.Time {
	switch drive.QuotaScope(err) {
	case "daily":
		return nextDailyQuotaReset(now)
	case "storage":
		return now.Add(storageQuotaRetry)
	}
	if d := drive.RetryAfterOf(err); d > 0 {
		return now.Add(d)
	}
	return now.Add(unknownQuotaRetry)
}

func (r *run) isQuota() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.quota
}

// blockQuota stops the whole run on an account-wide quota error. The item (when
// there is one) goes back to pending without spending an attempt, and the job
// becomes blocked with resume_after so the engine can continue it later.
func (r *run) blockQuota(it *state.Item, err error) {
	r.mu.Lock()
	already := r.quota
	r.quota = true
	r.mu.Unlock()

	now := r.e.Now().UTC()
	resume := quotaResume(err, now)
	scope := drive.QuotaScope(err)
	if it != nil {
		r.e.Store.ReleaseItemThrottled(r.db, *it, "quota_exceeded", resume.UnixMilli())
	}
	if already {
		return
	}
	msg := "The Google account reached its copy quota. The job will continue after " + resume.Format(time.RFC3339) + "."
	if scope == "storage" {
		msg = "The Google account has no storage space left. The job is blocked and checks again after " + resume.Format(time.RFC3339) + "."
	}
	if _, err := r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase = state.JobBlocked, nil
		j.LastError = &state.Failure{Code: "quota_exceeded", Message: msg}
		j.ResumeAfter = &resume
		return "blocked", true
	}); err == nil {
		r.log("blocked on an account quota", "resume_after", resume, "scope", scope)
	}
}
