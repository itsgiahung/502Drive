package state

import (
	"testing"
	"time"
)

func TestQuotaResumeOnlyUsesPrincipalQuotaJobs(t *testing.T) {
	s := openStore(t)
	j := queueJob(t, s, 1, "")
	now := time.Now()
	future := now.Add(time.Hour)
	_, err := s.UpdateJob(t.Context(), j.ID, func(j *Job) (string, bool) {
		j.State = JobBlocked
		j.LastError = &Failure{Code: "quota_exceeded"}
		j.ResumeAfter = &future
		return "blocked", true
	})
	if err != nil {
		t.Fatal(err)
	}
	at, err := s.QuotaResume(t.Context(), "other", now)
	if err != nil || at != nil {
		t.Fatalf("leaked other principal: %v %v", at, err)
	}
	at, err = s.QuotaResume(t.Context(), "owner", now)
	if err != nil || at == nil || !at.Equal(future) {
		t.Fatalf("resume=%v err=%v", at, err)
	}
}
