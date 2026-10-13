package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/proc"
	"github.com/yetone/magpie/internal/provider"
)

// openInBrowser opens url in the user's browser. It is a variable so the
// tests can have it go nowhere without starting a program (noBrowser).
var openInBrowser = openURLInBrowser

func openURLInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = proc.Command("open", url)
	case "windows":
		cmd = proc.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "android":
		cmd = proc.Command("termux-open-url", url)
	default:
		cmd = proc.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

type quotaSpan = provider.QuotaSpan

// quotaCell is one window in a line as of now: "5h 42% ↻2h13m 14:13".
func quotaCell(w quotaSpan, now time.Time) string {
	name := strings.NewReplacer(" hours", "h", " hour", "h", " days", "d", " day", "d", " · ", " ").Replace(w.Name)
	if w.Unlimited {
		// no cap is not 0% of one: a remote magpie's gateway key held to
		// no limit (#1547) read "Limit 0%"
		return name + " unlimited"
	}
	cell := fmt.Sprintf("%s %.0f%%", name, w.Used)
	if w.Display != "" {
		cell += " (" + w.Display + ")"
	}
	if w.ResetsAt != nil {
		if !w.ResetsAt.After(now) {
			cell += muted.Render(" · reset time passed " + w.ResetsAt.Local().Format("Jan 2 15:04"))
		} else {
			cell += muted.Render(" ↻" + untilShort(w.ResetsAt.Sub(now)) + " " + provider.ResetClock(*w.ResetsAt, now))
		}
	}
	return cell
}

// quotaReadingCell says when a line was read: "as of … (cached)" for a
// reading kept in place of one that failed, "read 12m ago" for one a
// cache handed back (a Claude account's waits for Claude Code to tell it
// again), nothing for one just read.
func quotaReadingCell(at, readAt *time.Time, ws []quotaSpan) string {
	if at == nil {
		if readAt != nil && time.Since(*readAt) >= 2*time.Minute {
			return "  " + muted.Render("read "+ago(*readAt))
		}
		return ""
	}
	cell := "  " + muted.Render("as of "+at.Local().Format("Jan 2 15:04")+" (cached)")
	if slices.ContainsFunc(ws, func(w quotaSpan) bool { return w.ResetsAt != nil && !w.ResetsAt.After(time.Now()) }) {
		cell += muted.Render(" · expired window; current allowance unknown")
	}
	return cell
}

// untilShort is a time left as "45m", "2h13m" or "3d4h".
func untilShort(d time.Duration) string {
	switch {
	case d <= 0:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}
