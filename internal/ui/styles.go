package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const dateLayout = "2006-01-02"

var (
	colorStarted  = lipgloss.AdaptiveColor{Light: "28", Dark: "10"}
	colorFinished = lipgloss.AdaptiveColor{Light: "248", Dark: "244"}
	colorOverdue  = lipgloss.AdaptiveColor{Light: "160", Dark: "9"}
	colorDueSoon  = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	colorAccent   = lipgloss.AdaptiveColor{Light: "25", Dark: "75"}
	colorSelected = lipgloss.AdaptiveColor{Light: "254", Dark: "237"}
	colorDim      = lipgloss.AdaptiveColor{Light: "250", Dark: "240"}

	styleHeader  = lipgloss.NewStyle().Bold(true).Underline(true)
	styleHelp    = lipgloss.NewStyle().Foreground(colorFinished)
	styleError   = lipgloss.NewStyle().Foreground(colorOverdue).Bold(true)
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleLabel   = lipgloss.NewStyle().Bold(true)
	styleFocused = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleMuted   = lipgloss.NewStyle().Foreground(colorFinished)
	styleBox     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
	styleField        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorDim)
	styleFieldFocused = styleField.BorderForeground(colorAccent)
	styleButton       = lipgloss.NewStyle().Padding(0, 2).Border(lipgloss.RoundedBorder()).BorderForeground(colorDim)
	styleButtonActive = styleButton.BorderForeground(colorAccent).Foreground(colorAccent).Bold(true)
)

// boxWidth is the outer width of an overlay box for the given screen width.
func boxWidth(screen, limit int) int {
	return max(24, min(screen-4, limit))
}

// boxInner is the usable content width inside an overlay box of outer width w.
func boxInner(w int) int {
	return w - styleBox.GetHorizontalFrameSize()
}

// fit truncates s to width cells and pads it with spaces to exactly width.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

// dim strips all styling from a rendered view and draws it faded, so an
// overlay on top of it stands out.
func dim(view string) string {
	lines := strings.Split(ansi.Strip(view), "\n")
	style := lipgloss.NewStyle().Foreground(colorDim)
	for i, l := range lines {
		lines[i] = style.Render(l)
	}
	return strings.Join(lines, "\n")
}

// placeOverlay draws fg centered on top of bg, which is expanded to
// width x height if necessary.
func placeOverlay(bg, fg string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	fgLines := strings.Split(fg, "\n")
	fw := lipgloss.Width(fg)
	x := max(0, (width-fw)/2)
	y := max(0, (height-len(fgLines))/2)
	for i, line := range fgLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		b := bgLines[row]
		if w := ansi.StringWidth(b); w < x+fw {
			b += strings.Repeat(" ", x+fw-w)
		}
		bgLines[row] = ansi.Truncate(b, x, "") + ansi.ResetStyle + line + ansi.ResetStyle + ansi.TruncateLeft(b, x+fw, "")
	}
	return strings.Join(bgLines, "\n")
}
