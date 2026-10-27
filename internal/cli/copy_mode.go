package cli

import (
	"bufio"
	"fmt"
	"strings"
)

// copyMode maps the flags to the L2 answer. Without a terminal and without a
// flag the answer is update: a second copy of the same source and destination
// refreshes the first one instead of duplicating it.
func copyMode(update, newCopy, interactive bool) string {
	switch {
	case update:
		return "update"
	case newCopy:
		return "new"
	case interactive:
		return ""
	}
	return "update"
}

// askExistingCopy asks the user what to do with an existing copy. Cancelling
// changes nothing on the server.
func (a *App) askExistingCopy() (string, error) {
	fmt.Fprint(a.errw(), "This source is already copied to this destination. [u]pdate it, make a [n]ew copy, or [c]ancel? ")
	line, _ := bufio.NewReader(a.io.In).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "u", "update":
		return "update", nil
	case "n", "new":
		return "new", nil
	}
	return "", localErr(ExitCancelled, "cancelled", "Copy cancelled; nothing was changed.")
}
