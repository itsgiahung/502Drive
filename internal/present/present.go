package present

import (
	"fmt"
	"strings"
	"time"
)

// Counts holds progress counters for a job or operation.
type Counts struct {
	Observed  int
	Total     *int
	Completed int
	Failed    int
	Blocked   int
	Skipped   int
	Bytes     int64
}

// JobSummary captures key metadata for displaying a job in a single line.
type JobSummary struct {
	ID     string
	State  string
	Name   string
	Counts Counts
	Rate   float64
	ETA    time.Duration
}

var statesVi = map[string]string{
	"queued":             "Đã xếp hàng",
	"running":            "Đang sao chép",
	"pausing":            "Đang dừng",
	"paused":             "Đã tạm dừng",
	"cancelling":         "Đang huỷ",
	"reconciling":        "Đang xác minh",
	"blocked":            "Cần xử lý",
	"succeeded":          "Xong",
	"partial":            "Xong một phần",
	"failed":             "Thất bại",
	"cancelled":          "Đã huỷ",
	"idle":               "Rảnh",
	"busy":               "Đang sao chép",
	"error":              "Lỗi",
	"offline":            "Ngoại tuyến",
	"sleeping":           "Đang ngủ",
	"suspending":         "Đang chuẩn bị ngủ",
	"reconnecting":       "Đang kết nối lại",
	"resuming":           "Đang kết nối lại",
	"reconnect_required": "Cần đăng nhập lại Google",
	"retired":            "Đã ngừng theo dõi",
}

var statesEn = map[string]string{
	"queued":             "Queued",
	"running":            "Copying",
	"pausing":            "Stop requested",
	"paused":             "Paused",
	"cancelling":         "Cancel requested",
	"reconciling":        "Verifying",
	"blocked":            "Needs action",
	"succeeded":          "Done",
	"partial":            "Partly done",
	"failed":             "Failed",
	"cancelled":          "Cancelled",
	"idle":               "Idle",
	"busy":               "Busy",
	"error":              "Error",
	"offline":            "Offline",
	"sleeping":           "Sleeping",
	"suspending":         "Suspending",
	"reconnecting":       "Reconnecting",
	"resuming":           "Reconnecting",
	"reconnect_required": "Sign in to Google again",
	"retired":            "Retired",
}

// StateLabel maps a job or service state code to a localized display label.
func StateLabel(state, lang string) string {
	raw := strings.TrimSpace(state)
	norm := strings.ToLower(raw)
	isVi := strings.HasPrefix(strings.ToLower(lang), "vi")

	if isVi {
		if label, ok := statesVi[norm]; ok {
			return label
		}
	} else {
		if label, ok := statesEn[norm]; ok {
			return label
		}
	}
	return raw
}

// HumanBytes converts raw byte count into a human-readable size string.
func HumanBytes(n int64) string {
	if n < 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	div := int64(unit)
	exp := 0
	for v := n / unit; v >= unit && exp < len(units)-1; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

// HumanDuration formats a duration into a compact human-readable representation.
func HumanDuration(d time.Duration, lang string) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	isVi := strings.HasPrefix(strings.ToLower(lang), "vi")

	if isVi {
		switch {
		case d >= time.Hour:
			return fmt.Sprintf("%dg%02dp", int(d.Hours()), int(d.Minutes())%60)
		case d >= time.Minute:
			return fmt.Sprintf("%dp%02ds", int(d.Minutes()), int(d.Seconds())%60)
		default:
			return fmt.Sprintf("%ds", int(d.Seconds()))
		}
	}

	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// Progress renders a compact summary of completion count, transfer rate, and ETA.
func Progress(counts Counts, rate float64, eta time.Duration, lang string) string {
	isVi := strings.HasPrefix(strings.ToLower(lang), "vi")
	var parts []string

	if counts.Total != nil {
		parts = append(parts, fmt.Sprintf("%d/%d", counts.Completed, *counts.Total))
	} else {
		if isVi {
			parts = append(parts, fmt.Sprintf("%d mục", counts.Completed))
		} else {
			parts = append(parts, fmt.Sprintf("%d items", counts.Completed))
		}
	}

	if rate > 0 {
		if isVi {
			parts = append(parts, fmt.Sprintf("%.1f/giây", rate))
		} else {
			parts = append(parts, fmt.Sprintf("%.1f/s", rate))
		}
	}

	if eta > 0 {
		durStr := HumanDuration(eta, lang)
		if isVi {
			parts = append(parts, fmt.Sprintf("còn %s", durStr))
		} else {
			parts = append(parts, fmt.Sprintf("ETA %s", durStr))
		}
	}

	if counts.Failed > 0 {
		if isVi {
			parts = append(parts, fmt.Sprintf("%d thất bại", counts.Failed))
		} else {
			parts = append(parts, fmt.Sprintf("%d failed", counts.Failed))
		}
	}

	if counts.Blocked > 0 {
		if isVi {
			parts = append(parts, fmt.Sprintf("%d bị chặn", counts.Blocked))
		} else {
			parts = append(parts, fmt.Sprintf("%d blocked", counts.Blocked))
		}
	}

	return strings.Join(parts, " · ")
}

// OneLine renders a single-line summary of a job suitable for menus, TUI rows, and logs.
func OneLine(job JobSummary, lang string) string {
	stateStr := StateLabel(job.State, lang)
	name := job.Name
	if name == "" {
		name = job.ID
	}
	prog := Progress(job.Counts, job.Rate, job.ETA, lang)
	return fmt.Sprintf("[%s] %s — %s", stateStr, name, prog)
}

// Reason preserves the CLI's English explanation entry point.
func Reason(code string) string { return Explain(code, "en").Message }
