package telegram

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The problem report (X1) as a file in the chat: the failed, blocked and
// skipped items of one job as CSV, with the reasons and what to do about them.
// It is the same data and columns as `502drive jobs report --format csv`, read
// through the same application service (JobItems), so a caller sees only the
// items of jobs they may see. The file holds Drive ids and paths relative to
// the copied folder and the stable codes; it never carries a path on the
// daemon's machine, a token or a server message.

const (
	// reportMaxRows matches the CLI export cap.
	reportMaxRows = 5000
	// reportMaxBytes keeps the file far under Telegram's upload limit.
	reportMaxBytes = 2 << 20
	// reportCaptionMax is Telegram's caption limit (1024) with some room.
	reportCaptionMax = 1000
	reportHintLines  = 3
)

// hasProblems says whether a job has items worth a report.
func hasProblems(j state.Job) bool {
	if j.Kind != "" {
		return false
	}
	c := j.Counts
	return c.Failed+c.Blocked+c.Skipped > 0
}

// csvCell neutralises a cell a spreadsheet would run as a formula (the same
// rule as the CLI export): the apostrophe is literal data.
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

type problemSummary struct {
	file      []byte
	rows      int
	truncated bool
	byCode    map[string]int
}

// buildProblemCSV pages the problem items of a job through the service and
// writes the CSV, stopping at the row and size caps.
func (t *Telegram) buildProblemCSV(ctx context.Context, p app.Principal, jobID string) (problemSummary, error) {
	out := problemSummary{byCode: map[string]int{}}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"state", "code", "reason", "path", "attempts", "source_id", "destination_id"})
	cursor := int64(0)
	for !out.truncated {
		items, next, err := t.svc.JobItems(ctx, p, jobID, true, 100, cursor)
		if err != nil {
			return out, err
		}
		for _, it := range items {
			if out.rows >= reportMaxRows || buf.Len() >= reportMaxBytes {
				out.truncated = true
				break
			}
			path := it.RelPath
			if path == "" {
				path = it.Name
			}
			row := []string{it.State, it.ErrorCode, present.Reason(it.ErrorCode), path,
				strconv.Itoa(it.Attempts), it.SourceID, it.DestID}
			for i := range row {
				row[i] = csvCell(row[i])
			}
			_ = w.Write(row)
			w.Flush()
			out.rows++
			code := it.ErrorCode
			if code == "" {
				code = "unknown"
			}
			out.byCode[code]++
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return out, err
	}
	out.file = buf.Bytes()
	return out, nil
}

// problemCaption is the file's caption: what it is, whether it was cut, and
// the advice for the most frequent reasons.
func problemCaption(tx texts, j state.Job, s problemSummary) string {
	ig, g := tx.ig(), tx.g()
	var b strings.Builder
	fmt.Fprintf(&b, ig.reportCaption+"\n", escape(short(j.RootName, 40)), s.rows)
	if s.truncated {
		fmt.Fprintf(&b, ig.reportTruncated+"\n", s.rows)
	}
	type kv struct {
		code string
		n    int
	}
	rows := make([]kv, 0, len(s.byCode))
	for code, n := range s.byCode {
		rows = append(rows, kv{code, n})
	}
	sort.Slice(rows, func(i, k int) bool {
		if rows[i].n != rows[k].n {
			return rows[i].n > rows[k].n
		}
		return rows[i].code < rows[k].code
	})
	if len(rows) > 0 {
		fmt.Fprintf(&b, "\n%s\n", ig.reportHow)
	}
	for i, r := range rows {
		if i == reportHintLines {
			break
		}
		label := g.skipLabels[r.code]
		if label == "" {
			label = codeText(tx, r.code)
		}
		hint := ig.itemHint[r.code]
		if hint == "" {
			hint = g.errHint[r.code]
		}
		if hint == "" {
			hint = ig.itemHintDefault
		}
		line := fmt.Sprintf(ig.reportHintLine+"\n", escape(label), escape(r.code), r.n, escape(hint))
		if len([]rune(b.String()))+len([]rune(line)) > reportCaptionMax {
			break
		}
		b.WriteString(line)
	}
	return strings.TrimRight(b.String(), "\n")
}

// actJobReport sends the CSV of one job. The button is one-shot and the job is
// read in the caller's scope, so another person's job answers "not found".
func (t *Telegram) actJobReport(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	ig := tx.ig()
	j, err := t.svc.Job(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	s, err := t.buildProblemCSV(ctx, p, j.ID)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if s.rows == 0 {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, ig.reportNone,
			[]button{act(tx.details, "job", j.ID, j.Revision, ""), nav(tx.back, "home")})
	}
	_, err = t.bot.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID:    st.ChatID,
		Document:  &models.InputFileUpload{Filename: "report-" + j.ID + ".csv", Data: bytes.NewReader(s.file)},
		Caption:   problemCaption(tx, j, s),
		ParseMode: models.ParseModeHTML,
	})
	return err
}
