package present

import (
	"strings"
	"testing"
	"time"
)

func TestStateLabel(t *testing.T) {
	tests := []struct {
		state string
		lang  string
		want  string
	}{
		// Vietnamese
		{"queued", "vi", "Đã xếp hàng"},
		{"running", "vi", "Đang sao chép"},
		{"pausing", "vi", "Đang dừng"},
		{"paused", "vi", "Đã tạm dừng"},
		{"cancelling", "vi", "Đang huỷ"},
		{"reconciling", "vi", "Đang xác minh"},
		{"blocked", "vi", "Cần xử lý"},
		{"succeeded", "vi", "Xong"},
		{"partial", "vi", "Xong một phần"},
		{"failed", "vi", "Thất bại"},
		{"cancelled", "vi", "Đã huỷ"},
		{"idle", "vi", "Rảnh"},
		{"busy", "vi", "Đang sao chép"},
		{"error", "vi", "Lỗi"},
		{"offline", "vi", "Ngoại tuyến"},
		{"unknown_state", "vi", "unknown_state"},

		// English
		{"queued", "en", "Queued"},
		{"running", "en", "Copying"},
		{"pausing", "en", "Stop requested"},
		{"paused", "en", "Paused"},
		{"cancelling", "en", "Cancel requested"},
		{"reconciling", "en", "Verifying"},
		{"blocked", "en", "Needs action"},
		{"succeeded", "en", "Done"},
		{"partial", "en", "Partly done"},
		{"failed", "en", "Failed"},
		{"cancelled", "en", "Cancelled"},
		{"idle", "en", "Idle"},
		{"busy", "en", "Busy"},
		{"error", "en", "Error"},
		{"offline", "en", "Offline"},
		{"unknown_state", "en", "unknown_state"},

		// Case-insensitivity and trimming
		{"  RUNNING  ", "en", "Copying"},
		{"Paused ", "vi", "Đã tạm dừng"},
	}

	for _, tt := range tests {
		got := StateLabel(tt.state, tt.lang)
		if got != tt.want {
			t.Errorf("StateLabel(%q, %q) = %q, want %q", tt.state, tt.lang, got, tt.want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{-1, "-"},
		{-100, "-"},
		{0, "0 B"},
		{1, "1 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{15728640, "15.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		got := HumanBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		lang string
		want string
	}{
		{0, "en", "0s"},
		{0, "vi", "0s"},
		{45 * time.Second, "en", "45s"},
		{45 * time.Second, "vi", "45s"},
		{2*time.Minute + 30*time.Second, "en", "2m30s"},
		{2*time.Minute + 30*time.Second, "vi", "2p30s"},
		{1*time.Hour + 5*time.Minute, "en", "1h05m"},
		{1*time.Hour + 5*time.Minute, "vi", "1g05p"},
		{25*time.Hour + 10*time.Minute, "en", "25h10m"},
		{25*time.Hour + 10*time.Minute, "vi", "25g10p"},
	}

	for _, tt := range tests {
		got := HumanDuration(tt.d, tt.lang)
		if got != tt.want {
			t.Errorf("HumanDuration(%v, %q) = %q, want %q", tt.d, tt.lang, got, tt.want)
		}
	}
}

func TestProgress(t *testing.T) {
	total50 := 50
	tests := []struct {
		name    string
		counts  Counts
		rate    float64
		eta     time.Duration
		lang    string
		wantSub []string
	}{
		{
			name: "completed with known total en",
			counts: Counts{
				Completed: 12,
				Total:     &total50,
			},
			rate:    5.2,
			eta:     2*time.Minute + 30*time.Second,
			lang:    "en",
			wantSub: []string{"12/50", "5.2/s", "2m30s"},
		},
		{
			name: "completed with known total vi",
			counts: Counts{
				Completed: 12,
				Total:     &total50,
			},
			rate:    5.2,
			eta:     2*time.Minute + 30*time.Second,
			lang:    "vi",
			wantSub: []string{"12/50", "5.2/giây", "còn 2p30s"},
		},
		{
			name: "unknown total without rate",
			counts: Counts{
				Completed: 8,
				Total:     nil,
			},
			rate:    0,
			eta:     0,
			lang:    "en",
			wantSub: []string{"8 items"},
		},
		{
			name: "unknown total vi",
			counts: Counts{
				Completed: 8,
				Total:     nil,
			},
			rate:    0,
			eta:     0,
			lang:    "vi",
			wantSub: []string{"8 mục"},
		},
		{
			name: "with failed and blocked items en",
			counts: Counts{
				Completed: 10,
				Total:     &total50,
				Failed:    2,
				Blocked:   1,
			},
			rate:    0,
			eta:     0,
			lang:    "en",
			wantSub: []string{"10/50", "2 failed", "1 blocked"},
		},
	}

	for _, tt := range tests {
		got := Progress(tt.counts, tt.rate, tt.eta, tt.lang)
		for _, sub := range tt.wantSub {
			if !strings.Contains(got, sub) {
				t.Errorf("%s: Progress() = %q, missing substring %q", tt.name, got, sub)
			}
		}
	}
}

func TestOneLine(t *testing.T) {
	total := 100
	job := JobSummary{
		ID:    "job_test_123",
		State: "running",
		Name:  "Documents/Backup",
		Counts: Counts{
			Completed: 25,
			Total:     &total,
		},
		Rate: 10.0,
		ETA:  30 * time.Second,
	}

	gotVi := OneLine(job, "vi")
	if !strings.Contains(gotVi, "Đang sao chép") || !strings.Contains(gotVi, "Documents/Backup") || !strings.Contains(gotVi, "25/100") {
		t.Errorf("OneLine vi unexpected: %q", gotVi)
	}

	gotEn := OneLine(job, "en")
	if !strings.Contains(gotEn, "Copying") || !strings.Contains(gotEn, "Documents/Backup") || !strings.Contains(gotEn, "25/100") {
		t.Errorf("OneLine en unexpected: %q", gotEn)
	}
}
