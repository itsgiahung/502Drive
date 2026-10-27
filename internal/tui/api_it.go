package tui

import "time"

// Types of batch IT: batch copy and the queue (S6), the deep preview (S7), the
// after-copy sync choice (S8), the problem report (X1) and the server-side job
// filter. They carry what the daemon answered; the model decides nothing here.

// After-copy sync modes (S8), the values of the daemon preference.
const (
	afterOff       = "off"
	afterRealtime  = "realtime"
	afterHourly    = "hourly"
	afterSixHourly = "six_hourly"
	afterDaily     = "daily"
)

// afterModes is the order the s key and the Settings row step through.
var afterModes = []string{afterOff, afterRealtime, afterHourly, afterSixHourly, afterDaily}

// DeepScan is the deep preflight block of a plan (S7). Complete is true only
// when the scan finished; until then the counts are "so far", never a total.
type DeepScan struct {
	State                                     string // running, complete, cancelled, expired, interrupted, failed
	Complete                                  bool
	Folders, Files                            int
	Bytes                                     int64
	Native, Shortcuts, CannotCopy, Unreadable int
	Scanned                                   int
	EstimateSeconds                           *int64
	Warnings                                  []string
}

// Running reports a scan that is still going.
func (d DeepScan) Running() bool { return d.State == "running" }

// SubmitOptions are the choices that travel with the job: Mode answers an
// existing copy ("", update or new) and AfterSync is the S8 sync mode for the
// new collection ("" lets the daemon use the preference).
type SubmitOptions struct {
	Mode      string
	AfterSync string
}

// BatchRequest copies every link to one destination under one batch.
type BatchRequest struct {
	Sources       []string
	DestinationID string
	AutoName      bool
	Options       CopyOptions
	Submit        SubmitOptions
}

// BatchJob is one link of a batch and what became of it.
type BatchJob struct {
	Source, JobID, Error string
}

// BatchResult is the answer to SubmitBatch.
type BatchResult struct {
	ID   string
	Jobs []BatchJob
}

// Counts returns how many links were accepted and how many were refused.
func (r BatchResult) Counts() (accepted, failed int) {
	for _, j := range r.Jobs {
		if j.JobID != "" {
			accepted++
		} else {
			failed++
		}
	}
	return accepted, failed
}

// BatchProgress is GET /v1/batches/{id}: its jobs by state.
type BatchProgress struct {
	ID            string
	Total         int
	CompletedJobs int
	ByState       map[string]int
}

// JobMove asks the daemon to place a queued job before or after another one
// and/or to change its priority.
type JobMove struct {
	Before, After string
	Priority      *int
}

// JobQuery is the server-side filter of the Jobs list. Pages is how many
// pages of jobsPage rows the snapshot reads (>= 1).
type JobQuery struct {
	State        string
	CollectionID string
	Pages        int
}

const jobsPage = 20

// ProblemItem is one failed, blocked or skipped item of a job (X1).
type ProblemItem struct {
	State, Code, Path string
	Attempts          int
}

// JobReport is the problem items of one job, cut at the daemon's row limit.
type JobReport struct {
	Items     []ProblemItem
	Truncated bool
}

// CodedError is a refusal with a stable code the TUI can word itself.
type CodedError struct{ Code, Message string }

func (e *CodedError) Error() string { return e.Message }

// batchState is the batch progress the model last read.
type batchState struct {
	prog    *BatchProgress
	at      time.Time
	loading bool
}

// lastAuditState is the newest audit of a collection the model last read.
type lastAuditState struct {
	rep     *AuditReport
	at      time.Time
	loading bool
}
