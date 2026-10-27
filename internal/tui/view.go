package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/mascot"
	"github.com/itsgiahung/502Drive/internal/present"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸"}
var spinFramesASCII = []string{"|", "/", "-", "\\"}

// View renders the whole screen. It is pure: the same model always renders the
// same text, which is what the tests assert. When the terminal height is known
// the output is exactly that many lines (the body is windowed and padded), so
// a shorter screen never leaves lines of the previous one behind.
func (m Model) View() string {
	if m.screen == screenHome && m.snap.Setup != nil && !m.snap.Setup.Ready {
		lines := []string{m.t(i18n.SetupTitle), ""}
		for _, c := range m.snap.Setup.Checks {
			mark := "✗"
			if c.Status == "passed" {
				mark = "✓"
			}
			if m.opt.ASCII {
				mark = "[ ]"
				if c.Status == "passed" {
					mark = "[x]"
				}
			}
			lines = append(lines, mark+" "+present.SetupStep(c.Name, m.lang))
		}
		return m.fitBlock(append(lines, "", m.t(i18n.HomeHint)))
	}
	if m.screen == screenHome {
		return m.homeFrame()
	}
	body := m.body()
	if m.height > 0 {
		body = clipPad(body, m.bodyRows())
	}
	lines := append([]string{m.tabBar()}, body...)
	lines = append(lines, m.statusLine(), m.footer())
	return m.fitBlock(lines)
}

func clipPad(lines []string, rows int) []string {
	if len(lines) > rows {
		return lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) statusLine() string {
	switch {
	case m.confirm != nil:
		return m.warn(m.confirm.prompt)
	case m.status != "":
		return m.dim(m.status)
	case len(m.selected) > 0:
		return m.accent(m.tf(i18n.Selected, len(m.selected)))
	}
	return ""
}

func (m Model) liveWord() string {
	if m.watching {
		return m.good("● " + m.t(i18n.Live))
	}
	if m.acts.Watch != nil {
		return m.warn("○ " + m.t(i18n.Reconnecting))
	}
	return m.good(m.t(i18n.Ready))
}

// countWord renders "N <label>" for one status counter.
func countWord(n int, label string) string {
	return fmt.Sprintf("%d %s", n, label)
}

func (m Model) accountWord(status string) string {
	switch status {
	case "connected":
		return m.good(status)
	case "reconnect_required":
		return m.warn(status)
	case "":
		return m.dim("unknown")
	}
	return m.bad(status)
}

func (m Model) body() []string {
	lines := m.screenBody()
	if m.exp != nil && m.screen != screenJobs && m.screen != screenReport {
		lines = append(lines, m.exportLines()...)
	}
	return lines
}

func (m Model) screenBody() []string {
	switch m.screen {
	case screenJobs:
		return m.jobsView()
	case screenJob:
		return m.jobView()
	case screenLibrary:
		return m.libraryView()
	case screenCollection:
		return m.collectionView()
	case screenDestination:
		return m.destinationView()
	case screenAccount:
		return m.accountView()
	case screenSettings:
		return m.settingsView()
	case screenHelp:
		return m.helpView()
	case screenDrive:
		return m.driveView()
	case screenCopy:
		return m.copyView()
	case screenReport:
		return m.reportScreen()
	}
	return m.homeView()
}

func (m Model) homeView() []string {
	lines := []string{""}
	lines = append(lines, m.mascotLines()...)
	lines = append(lines, "",
		m.t(i18n.Google)+": "+m.accountWord(m.snap.Status.AccountStatus)+"  ·  "+fmt.Sprintf("%s · %s · %s",
			countWord(m.snap.Status.Running, m.t(i18n.Running)),
			countWord(m.snap.Status.Queued, m.t(i18n.Queued)),
			countWord(m.snap.Status.Blocked, m.t(i18n.Blocked))),
		m.workLine(),
		"",
		"1 "+m.t(i18n.Home)+"   2 "+m.t(i18n.Jobs)+"   3 "+m.t(i18n.Library)+"   4 "+
			m.t(i18n.Destination)+"   5 "+m.t(i18n.Account)+"   6 "+m.t(i18n.Settings)+"   7 "+m.t(i18n.Drive)+"   c "+m.t(i18n.CopyTitle)+"   ? "+m.t(i18n.Help))
	if m.snap.Status.BotState != "" && m.snap.Status.BotState != "disabled" {
		lines = append(lines, "", m.dim(m.t(i18n.Telegram)+": "+Sanitize(m.snap.Status.BotState)))
	}
	if m.snap.Status.QueuePaused {
		lines = append(lines, "", m.warn(m.t(i18n.QueuePausedBanner)))
	}
	return lines
}

// workLine summarises what is happening: totals and speed of the running jobs,
// a greeting right after start, or a hint.
func (m Model) workLine() string {
	done, total, perSec := 0, 0, 0.0
	known := false
	for _, j := range m.snap.Jobs {
		if j.State != "running" || j.Total == nil {
			continue
		}
		known = true
		done += j.Completed
		total += *j.Total
		if p, _, ok := m.speed(j); ok {
			perSec += p
		}
	}
	switch {
	case known:
		text := m.tf(i18n.WorkingNow, done, total)
		if perSec > 0 {
			text += " · " + m.tf(i18n.FilesPerSec, perSec)
			if total > done {
				text += " · " + m.tf(i18n.ETALeft, m.humanDuration(time.Duration(float64(total-done)/perSec*float64(time.Second))))
			}
		}
		return m.accent(text)
	case m.snap.Status.Running > 0 || m.snap.Status.Queued > 0:
		return m.accent(m.tf(i18n.WorkingNow, 0, 0))
	case m.needsGoogle():
		return m.warn(m.t(i18n.LoginHome))
	case m.opt.now().Sub(m.started) < greetFor:
		return m.dim(m.greeting())
	case m.opt.now().Before(m.happyUntil):
		return m.good(m.t(i18n.AllDone))
	}
	return m.dim(m.t(i18n.HomeHint))
}

// needsGoogle reports a daemon that is up but has no usable Google account.
func (m Model) needsGoogle() bool {
	st := m.snap.Status.AccountStatus
	return m.online && m.acts.Login != nil && st != "" && st != "connected"
}

func (m Model) greeting() string {
	switch h := m.opt.now().Hour(); {
	case h < 11:
		return m.t(i18n.GreetMorning)
	case h < 18:
		return m.t(i18n.GreetDay)
	}
	return m.t(i18n.GreetEvening)
}

// mascotLines draws the animated cat when the terminal can show it, and a
// small text bot otherwise (plain output, --ascii, short or narrow terminals).
func (m Model) mascotLines() []string {
	mood := m.mood(m.opt.now())
	if m.opt.Color && !m.opt.ASCII && m.width >= 36 && (m.height == 0 || m.height >= mascot.Rows+11) {
		opt := mascot.Options{Profile: m.opt.Profile, Dark: m.opt.Dark, Crop: m.width < mascot.Cols+2}
		lines := m.frame.Lines(opt)
		if m.opt.NoMotion {
			lines = mascot.Static(mood, .4).Lines(opt)
		}
		return lines
	}
	return mascotArt(string(mood), m.opt.ASCII)
}

func mascotArt(mood string, ascii bool) []string {
	eyes := map[string]string{"idle": "o  o", "busy": "^  ^", "happy": "^  ^", "wait": "-  -", "sleep": "-  -", "error": "x  x"}[mood]
	if eyes == "" {
		eyes = "o  o"
	}
	if ascii {
		return []string{" .------.", " | " + eyes + " |", " |  --  |", " '------'"}
	}
	return []string{" ╭──────╮", " │ " + eyes + " │", " │  ──  │", " ╰──────╯"}
}

func (m Model) jobsView() []string {
	if len(m.snap.Jobs) == 0 {
		empty := m.t(i18n.NoJobs)
		if m.filterLine() != "" {
			empty = m.t(i18n.JobsNoMatch)
		}
		lines := append(append([]string{""}, m.lifeBanner()...), m.bold(m.t(i18n.Jobs)))
		lines = append(lines, m.jobsHeadLines()...)
		lines = append(lines, "", m.dim(empty))
		return append(lines, m.exportLines()...)
	}
	start, end := m.window()
	lines := append([]string{""}, m.jobsBanner()...)
	lines = append(lines, m.lifeBanner()...)
	lines = append(lines, m.bold(m.t(i18n.Jobs))+" "+m.dim(m.scrollLabel(start, end, len(m.snap.Jobs))))
	lines = append(lines, m.jobsHeadLines()...)
	barWidth := min(36, m.width-49)
	for i := start; i < end; i++ {
		j := m.snap.Jobs[i]
		name := cut(Sanitize(j.SourceName), max(8, m.width-30))
		row := fmt.Sprintf("%s%-10s %s", m.marker(i, j.ID), m.stateWord(j.State), name)
		if m.busyState(j.State) && !m.opt.NoMotion {
			row += " " + m.spinChar()
		}
		lines = append(lines, row)
		switch {
		case m.waitingForQuota(j):
			lines = append(lines, "            "+m.warn(m.quotaText(j))) // no bar and no retry: it resumes by itself
		case j.State == "queued":
			lines = append(lines, "            "+m.queueLine(j))
		default:
			row := m.progressRow(j, barWidth)
			if j.BatchID != "" && m.width >= 56 {
				row += m.dim(" · " + m.batchChip(j.BatchID))
			}
			lines = append(lines, "            "+row)
		}
	}
	if m.snap.JobsNext != "" && end == len(m.snap.Jobs) {
		lines = append(lines, m.dim(m.t(i18n.JobsMoreRows)))
	}
	return append(lines, m.exportLines()...)
}

// jobsMoreLines is the line that says the daemon has more jobs than are shown.
func (m Model) jobsMoreLines() int {
	if m.snap.JobsNext != "" {
		return 1
	}
	return 0
}

// marker is the cursor and selection column of a list row.
func (m Model) marker(i int, id string) string {
	cursor, mark := " ", " "
	if i == m.cursor {
		cursor = m.accent(">")
	}
	if m.selected[id] {
		mark = m.good("●")
	}
	return cursor + mark
}

func (m Model) scrollLabel(start, end, n int) string {
	if end-start >= n {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d (%s)", n, m.tf(i18n.ScrollPos, start+1, end, n))
}

// progressRow is the bar plus, for a running job, speed and time left.
func (m Model) progressRow(j Job, barWidth int) string {
	text := m.progress(j, barWidth)
	if perSec, eta, ok := m.speed(j); ok && barWidth >= 10 {
		text += m.dim(" · " + m.tf(i18n.FilesPerSec, perSec))
		if eta > 0 {
			text += m.dim(" · " + m.tf(i18n.ETALeft, m.humanDuration(eta)))
		}
	}
	return text
}

func (m Model) jobView() []string {
	j, ok := m.currentJob()
	if !ok {
		return []string{"", m.dim(m.t(i18n.SelectJob))}
	}
	lines := []string{"", m.bold(m.t(i18n.Job)+" ") + Sanitize(j.ID), ""}
	lines = append(lines, m.kv(m.t(i18n.Source), Sanitize(j.SourceName)), m.kv(m.t(i18n.State), m.stateWord(j.State)))
	if j.Kind != "" {
		lines = append(lines, m.kv(m.t(i18n.KindLabel), m.kindLabel(j.Kind)))
	}
	if m.waitingForQuota(j) {
		lines = append(lines, m.kv(m.t(i18n.Blocked), m.warn(m.quotaText(j))), m.kv("", m.dim(m.t(i18n.QuotaWaitNote))))
	}
	if j.Phase != "" {
		lines = append(lines, m.kv(m.t(i18n.Phase), Sanitize(j.Phase)))
	}
	if j.State == "queued" {
		lines = append(lines, m.kv(m.t(i18n.State), present.Queue(j.QueueAhead, m.snap.Status.QueuePaused, m.lang)))
	}
	if j.Priority != 0 {
		lines = append(lines, m.kv(m.t(i18n.PriorityLabel), signed(j.Priority)))
	}
	lines = append(lines, m.batchDetail(j)...)
	lines = append(lines, m.kv(m.t(i18n.Progress), m.progress(j, max(10, min(32, m.width-36)))))
	lines = append(lines, m.kv(m.t(i18n.Counts), fmt.Sprintf("%d · %d · %d · %d",
		j.Completed, j.Failed, j.Blocked, j.Skipped)))
	if j.Bytes > 0 {
		lines = append(lines, m.kv(m.t(i18n.SizeLabel), bytesText(j.Bytes)))
	}
	if j.ResultURL != "" {
		lines = append(lines, m.kv(m.t(i18n.Result), Sanitize(j.ResultURL)))
	}
	if j.LastError != "" {
		lines = append(lines, m.kv(m.t(i18n.Problem), m.warn(Sanitize(j.LastError))))
	}
	if skipped := m.skippedLines(j); len(skipped) > 0 {
		lines = append(lines, "")
		lines = append(lines, skipped...)
	}
	keys := i18n.JobKeys
	if m.waitingForQuota(j) {
		keys = i18n.FooterJobWait
	}
	lines = append(lines, "", m.dim(m.t(keys)))
	return lines
}

func (m Model) libraryView() []string {
	if len(m.snap.Collections) == 0 {
		lines := append([]string{""}, m.lifeBanner()...)
		lines = append(lines, m.bold(m.t(i18n.Library)))
		lines = append(lines, m.libraryNote()...)
		return append(lines, "", m.dim(m.t(i18n.NoSources)))
	}
	start, end := m.window()
	lines := append([]string{""}, m.lifeBanner()...)
	lines = append(lines, m.bold(m.t(i18n.Library))+" "+m.dim(m.scrollLabel(start, end, len(m.snap.Collections))))
	lines = append(lines, m.libraryNote()...)
	for i := start; i < end; i++ {
		c := m.snap.Collections[i]
		lines = append(lines, fmt.Sprintf("%s%-10s %s", m.marker(i, c.ID), m.stateWord(c.State), cut(Sanitize(c.SourceName), max(8, m.width-14))))
		if c.State == "retired" {
			lines = append(lines, "            "+m.dim(m.t(i18n.LibRetiredTag)))
			continue
		}
		line := m.syncLine(c)
		if chip := m.auditChip(c); chip != "" {
			line = chip + m.dim(" · ") + line
		}
		lines = append(lines, "            "+line)
	}
	return lines
}

// syncLine summarises the subscription, or says it is off.
func (m Model) syncLine(c Collection) string {
	if c.Sync == nil {
		return m.dim(m.t(i18n.Sync) + ": " + m.t(i18n.SyncOff))
	}
	text := m.t(i18n.Sync) + ": " + c.Sync.Mode + " · " + c.Sync.State
	if c.Sync.Schedule != "" {
		text += " · " + Sanitize(c.Sync.Schedule)
	}
	if c.Sync.NextRun != "" {
		text += " · " + m.t(i18n.NextRun) + " " + Sanitize(c.Sync.NextRun)
	}
	return m.dim(text)
}

func (m Model) collectionView() []string {
	c, ok := m.currentCollection()
	if !ok {
		return []string{"", m.dim(m.t(i18n.SelectSource))}
	}
	lines := []string{"", m.bold(Sanitize(c.SourceName)), ""}
	lines = append(lines, m.kv(m.t(i18n.CopyState), m.stateWord(c.State)))
	if c.Sync == nil {
		lines = append(lines, m.kv(m.t(i18n.Sync), m.t(i18n.SyncOff)))
	} else {
		lines = append(lines, m.kv(m.t(i18n.Sync), c.Sync.Mode+" · "+c.Sync.State))
		lines = append(lines, m.kv(m.t(i18n.Revision), fmt.Sprintf("%d", c.Sync.Revision)))
		if c.Sync.Schedule != "" {
			lines = append(lines, m.kv(m.t(i18n.Schedule), Sanitize(c.Sync.Schedule)))
		}
		if c.Sync.LastSuccess != "" {
			lines = append(lines, m.kv(m.t(i18n.LastSuccess), Sanitize(c.Sync.LastSuccess)))
		}
		if c.Sync.NextRun != "" {
			lines = append(lines, m.kv(m.t(i18n.NextRun), Sanitize(c.Sync.NextRun)))
		}
		if c.Sync.ActiveJob != "" {
			lines = append(lines, m.kv(m.t(i18n.ActiveJob), Sanitize(c.Sync.ActiveJob)))
		}
		if c.Sync.LastError != "" {
			lines = append(lines, m.kv(m.t(i18n.Problem), m.warn(Sanitize(c.Sync.LastError))))
		}
	}
	if c.ResultURL != "" {
		lines = append(lines, m.kv(m.t(i18n.Output), Sanitize(c.ResultURL)))
	}
	if c.State == "retired" {
		lines = append(lines, m.kv("", m.dim(m.t(i18n.LibRetiredTag))))
	}
	lines = append(lines, m.kv(m.t(i18n.FilterTitle), m.filterText(c.Filter)))
	if c.OnConflict != "" && c.OnConflict != "rename" {
		lines = append(lines, m.kv(m.t(i18n.ConflictTitle), m.conflictLabel(c.OnConflict)))
	}
	lines = append(lines, m.auditLines(c)...)
	lines = append(lines, m.cleanupLines(c)...)
	keys := m.t(i18n.CollKeys)
	if m.hasLibraryExt() {
		keys += " · " + m.t(i18n.CollKeysExt)
	}
	lines = append(lines, "")
	for _, l := range wrapLines(keys, max(20, m.width-2)) {
		lines = append(lines, m.dim(l))
	}
	return lines
}

func (m Model) destinationView() []string {
	if len(m.snap.Destinations) == 0 {
		return []string{"", m.bold(m.t(i18n.Destination)), "", m.dim(m.t(i18n.NoDest))}
	}
	lines := []string{"", m.bold(m.t(i18n.Destination))}
	start, end := m.window()
	for i := start; i < end; i++ {
		d := m.snap.Destinations[i]
		marker := "  "
		if i == m.cursor {
			marker = m.accent("> ")
		}
		row := marker + Sanitize(d.Name)
		if d.Default {
			row += " " + m.good("(default)")
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", m.dim(m.t(i18n.DestKeys)))
	return lines
}

func (m Model) accountView() []string {
	lines := []string{"", m.bold(m.t(i18n.Account)), ""}
	lines = append(lines, m.kv(m.t(i18n.Google), m.accountWord(m.snap.Account.Status)))
	if m.snap.Account.Label != "" {
		lines = append(lines, m.kv(m.t(i18n.Account), Sanitize(m.snap.Account.Label)))
	}
	if m.snap.Account.Problem != "" {
		lines = append(lines, m.kv(m.t(i18n.Problem), m.warn(Sanitize(m.snap.Account.Problem))))
	}
	lines = append(lines, m.kv(m.t(i18n.Telegram), Sanitize(orDash(m.snap.Status.BotState))))
	lines = append(lines, m.loginLines()...)
	if len(m.snap.Checks) == 0 {
		lines = append(lines, "", m.dim(m.t(i18n.AccountKeys)))
		return lines
	}
	lines = append(lines, "", m.bold(m.t(i18n.Checks)))
	for _, c := range m.snap.Checks {
		status := c.Status
		switch status {
		case "passed":
			status = m.good("passed")
		case "failed":
			status = m.bad("failed")
		default:
			status = m.dim(status)
		}
		lines = append(lines, fmt.Sprintf("%-8s %-14s %s", status, Sanitize(c.Name), Sanitize(c.Message)))
	}
	return lines
}

func (m Model) helpView() []string {
	lines := []string{"", m.bold(m.t(i18n.Help))}
	help := i18n.HelpLines(m.lang)
	start, end := m.window()
	for _, l := range help[start:end] {
		lines = append(lines, "  "+Sanitize(l))
	}
	return lines
}

func (m Model) footer() string {
	keys := m.t(i18n.FooterKeys)
	switch m.screen {
	case screenHome:
		keys = m.t(i18n.FooterHome)
	case screenJob:
		keys = m.t(i18n.FooterJob)
		if j, ok := m.currentJob(); ok && m.waitingForQuota(j) {
			keys = m.t(i18n.FooterJobWait)
		}
	case screenCollection:
		keys = m.t(i18n.FooterColl)
		if m.hasLibraryExt() {
			keys = m.t(i18n.FooterCollExt)
		}
	case screenJobs:
		keys = m.t(i18n.FooterList)
		if m.hasJobsExt() {
			keys = m.t(i18n.JobsKeysIt)
		}
	case screenReport:
		keys = m.t(i18n.ReportKeys)
	case screenLibrary:
		keys = m.t(i18n.FooterLib)
		if m.acts.ShowRetired != nil {
			keys = m.t(i18n.FooterLibExt)
		}
	case screenDrive:
		keys = m.t(i18n.FooterDrive)
		if m.drv != nil && m.drv.pick {
			keys = m.t(i18n.FooterPick)
		}
		if m.drv != nil && m.drv.query != "" {
			keys = m.t(i18n.SearchKeys)
		}
	case screenCopy:
		keys = m.t(i18n.FooterCopyPlan)
		if m.cp != nil && m.cp.isBatch() {
			keys = m.t(i18n.BatchKeys)
		}
		if m.cp != nil && m.cp.input != nil {
			keys = m.t(i18n.FooterCopySrc)
		}
		if m.cp != nil && m.cp.choose {
			keys = m.t(i18n.ExistingCopyKeys)
		}
	case screenHelp:
		keys = m.t(i18n.FooterHelp)
	}
	if m.tabFocus {
		keys = m.t(i18n.TabFocusHint)
	}
	return m.dim(keys)
}

// progress draws a real bar from counts; an unknown total never shows a fake
// percentage.
func (m Model) progress(j Job, width int) string {
	if j.Total == nil || *j.Total <= 0 {
		return m.dim(i18n.Tf(m.lang, i18n.ItemsUnknown, j.Completed))
	}
	total := *j.Total
	done := min(j.Completed, total)
	if width < 10 {
		return fmt.Sprintf("%d/%d", done, total)
	}
	frac := float64(done) / float64(total)
	return m.bar(frac, width) + fmt.Sprintf(" %3d%% %d/%d", int(frac*100), done, total)
}

var partialBlocks = []rune(" ▏▎▍▌▋▊▉")

// bar draws width cells with eighth-block resolution, so progress moves
// smoothly instead of in whole-cell steps.
func (m Model) bar(frac float64, width int) string {
	frac = max(0, min(1, frac))
	if m.opt.ASCII {
		fill := int(frac * float64(width))
		return "[" + strings.Repeat("#", fill) + strings.Repeat("-", width-fill) + "]"
	}
	eighths := int(frac*float64(width)*8 + .5)
	full, part := eighths/8, eighths%8
	var sb strings.Builder
	for i := 0; i < full; i++ {
		sb.WriteString(m.gradient(i, width, "█"))
	}
	used := full
	if part > 0 && full < width {
		sb.WriteString(m.gradient(full, width, string(partialBlocks[part])))
		used++
	}
	rest := strings.Repeat("░", width-used)
	if m.opt.Color {
		rest = m.dim(rest)
	}
	return sb.String() + rest
}

// gradient colours cell i of n from cyan to violet.
func (m Model) gradient(i, n int, glyph string) string {
	if !m.opt.Color {
		return glyph
	}
	t := float64(i) / float64(max(1, n-1))
	r, g, b := int(80+90*t), int(200-80*t), 255
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))).Render(glyph)
}

func (m Model) kv(key, value string) string {
	return m.dim(fmt.Sprintf("%-12s", key)) + " " + value
}

func (m Model) stateWord(state string) string {
	label := stateWord(state)
	if !m.opt.Color {
		return label
	}
	switch state {
	case "succeeded", "ready", "active", "connected", "passed":
		return m.good(label)
	case "partial", "blocked", "paused", "needs_action", "reconnect_required", "warning":
		return m.warn(label)
	case "failed", "cancelled", "error":
		return m.bad(label)
	}
	return m.accent(label)
}

func (m Model) busyState(state string) bool {
	switch state {
	case "running", "queued", "pausing", "cancelling", "reconciling", "copying":
		return true
	}
	return false
}

func (m Model) spinChar() string {
	if m.opt.ASCII {
		return spinFramesASCII[m.spin%len(spinFramesASCII)]
	}
	return spinFrames[m.spin%len(spinFrames)]
}

// fitBlock cuts every line to the terminal width. ansi.Truncate keeps colour
// codes intact and counts wide characters by display width, so neither an
// emoji nor a CJK name can push a line past the edge and wrap over the next.
func (m Model) fitBlock(lines []string) string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimRight(ansi.Truncate(line, width, ""), " "))
	}
	return strings.Join(out, "\n")
}

func visible(s string) int { return lipgloss.Width(s) }

func (m Model) accent(s string) string { return m.paint("39", true, s) }
func (m Model) good(s string) string   { return m.paint("42", false, s) }
func (m Model) warn(s string) string   { return m.paint("214", false, s) }
func (m Model) bad(s string) string    { return m.paint("203", false, s) }
func (m Model) dim(s string) string    { return m.paint("244", false, s) }
func (m Model) bold(s string) string {
	if !m.opt.Color {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Render(s)
}

// paintBG draws s with a background colour (used for the active tab).
func (m Model) paintBG(bg, fg, s string) string {
	if !m.opt.Color {
		return s
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Bold(true).Render(s)
}

func (m Model) paint(color string, bold bool, s string) string {
	if !m.opt.Color {
		return s
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if bold {
		style = style.Bold(true)
	}
	return style.Render(s)
}

// t translates one label for the model's current language.
func (m Model) t(key i18n.Key) string { return i18n.T(m.lang, key) }

// tf translates and formats one label.
func (m Model) tf(key i18n.Key, args ...any) string { return i18n.Tf(m.lang, key, args...) }

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
