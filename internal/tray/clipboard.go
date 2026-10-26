package tray

import (
	"context"
	"errors"
	"net/url"
	"runtime"
	"strings"

	"github.com/itsgiahung/502Drive/internal/drive"
)

func goos() string { return runtime.GOOS }

// clipboardCandidates are fixed argv per platform: no shell, no user text.
func clipboardCandidates(goos string) [][]string {
	switch goos {
	case "windows":
		return [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard"}}
	case "darwin":
		return [][]string{{"pbpaste"}}
	default:
		return [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "--clipboard", "--output"}}
	}
}

// ReadDriveLink returns the single Drive reference currently in the clipboard.
// Anything else (multiple links, arbitrary text) is refused before any request
// is sent.
func ReadDriveLink(ctx context.Context, h Host) (string, error) {
	var lastErr error
	for _, argv := range clipboardCandidates(h.GOOS) {
		out, ok := runHost(ctx, h, argv)
		if !ok {
			continue
		}
		link, err := parseClipboard(out)
		if err != nil {
			lastErr = err
			continue
		}
		return link, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("no clipboard tool found (tried wl-paste, xclip, xsel)")
}

// parseClipboard accepts exactly one Drive link or Drive ID.
func parseClipboard(text string) (string, error) {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			lines = append(lines, v)
		}
	}
	switch len(lines) {
	case 0:
		return "", errors.New("the clipboard is empty")
	case 1:
	default:
		return "", errors.New("the clipboard must hold exactly one Drive link")
	}
	ref, err := drive.Parse(lines[0])
	if err != nil {
		return "", errors.New("the clipboard does not hold a Drive link or Drive ID")
	}
	return canonicalLink(ref), nil
}

// canonicalLink rebuilds a Drive URL from a parsed reference, so the request
// never carries unrelated query parameters from the clipboard.
func canonicalLink(ref drive.Reference) string {
	link := "https://drive.google.com/open?id=" + ref.ID
	if ref.Key != "" {
		link += "&resourcekey=" + ref.Key
	}
	return link
}

// OpenURL opens an https Drive URL in the browser. The URL comes from the
// daemon, but its host is still checked before it reaches an argv.
func OpenURL(ctx context.Context, h Host, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return errors.New("refusing to open a URL that is not plain https")
	}
	if u.Hostname() != "drive.google.com" {
		return errors.New("refusing to open a non-Drive URL")
	}
	switch h.GOOS {
	case "windows":
		if _, ok := runHost(ctx, h, []string{"rundll32", "url.dll,FileProtocolHandler", target}); ok {
			return nil
		}
		return errors.New("cannot open the browser")
	case "darwin":
		if _, ok := runHost(ctx, h, []string{"open", target}); ok {
			return nil
		}
		return errors.New("cannot open the browser")
	default:
		for _, opener := range []string{"xdg-open", "gio"} {
			argv := []string{opener, target}
			if opener == "gio" {
				argv = []string{"gio", "open", target}
			}
			if _, ok := runHost(ctx, h, argv); ok {
				return nil
			}
		}
		return errors.New("cannot open the browser (install xdg-open)")
	}
}
