package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var sub2Style = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // amber yellow

type tickMsg time.Time

type model struct {
	subs        []Subtitle
	subs2       []Subtitle // optional second language track
	elapsed     time.Duration
	playing     bool
	lastTick    time.Time
	jumpMode    bool
	jumpBuf     string
	width       int
	height      int
	obscureMode bool         // when true, subs2 text is hidden by default
	revealed    map[int]bool // subs2 indices revealed while obscureMode is on
	mdPath      string       // output markdown file path (derived from first SRT)
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if m.playing {
			now := time.Time(msg)
			m.elapsed += now.Sub(m.lastTick)
			m.lastTick = now
			return m, doTick()
		}

	case tea.KeyMsg:
		if m.jumpMode {
			switch msg.String() {
			case "enter":
				if d, ok := parseJumpTimestamp(m.jumpBuf); ok {
					m.elapsed = d
				}
				m.jumpMode = false
				m.jumpBuf = ""
			case "esc":
				m.jumpMode = false
				m.jumpBuf = ""
			case "backspace":
				if len(m.jumpBuf) > 0 {
					runes := []rune(m.jumpBuf)
					m.jumpBuf = string(runes[:len(runes)-1])
				}
			default:
				if len(msg.Runes) > 0 {
					m.jumpBuf += string(msg.Runes)
				}
			}
			return m, nil
		}

		switch msg.String() {
		case " ":
			if !m.playing {
				m.lastTick = time.Now()
				m.playing = true
				return m, doTick()
			}
			m.playing = false

		case "right", "n":
			for _, s := range m.subs {
				if s.Start > m.elapsed {
					m.elapsed = s.Start
					break
				}
			}

		case "left", "p":
			idx := -1
			for i, s := range m.subs {
				if s.Start < m.elapsed {
					idx = i
				}
			}
			if idx >= 0 && m.elapsed == m.subs[idx].Start && idx > 0 {
				m.elapsed = m.subs[idx-1].Start
			} else if idx >= 0 {
				m.elapsed = m.subs[idx].Start
			} else {
				m.elapsed = 0
			}

		case "r":
			if m.obscureMode {
				cur2 := activeSubtitleIdx(m.subs2, m.elapsed)
				if cur2 >= 0 {
					m.revealed[cur2] = true
				}
			}

		case "R":
			m.obscureMode = !m.obscureMode
			if m.obscureMode {
				m.revealed = make(map[int]bool)
			}

		case "s":
			saveCurrent(m)

		case "t":
			m.jumpMode = true
			m.jumpBuf = ""

		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	stateLabel := "⏸ paused"
	if m.playing {
		stateLabel = "▶ playing"
	}
	statusLine := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf(" [%s]  %s", formatDuration(m.elapsed), stateLabel),
	)

	var mainArea, nextLine string

	if len(m.subs2) > 0 {
		mainArea = m.renderDualColumns(w)
	} else {
		innerW := max(w-6, 10)
		curIdx := activeSubtitleIdx(m.subs, m.elapsed)
		var boxContent string
		if curIdx >= 0 {
			boxContent = m.subs[curIdx].Text
		} else {
			boxContent = " "
		}
		mainArea = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Width(innerW).
			Padding(1, 2).
			Align(lipgloss.Center).
			Render(boxContent)

		nextIdx := nextSubtitleIdx(m.subs, m.elapsed)
		if nextIdx >= 0 {
			next := firstLine(m.subs[nextIdx].Text)
			nextLine = lipgloss.NewStyle().Faint(true).Render(
				fmt.Sprintf(" next › %s", truncateLine(next, w-12)),
			)
		}
	}

	var bottomLine string
	if m.jumpMode {
		bottomLine = fmt.Sprintf(" jump to: %s█", m.jumpBuf)
	} else {
		help := " space play/pause  ←→/n/p prev/next  t jump  s save  q quit"
		if len(m.subs2) > 0 {
			if m.obscureMode {
				help += "  r reveal  R show-all"
			} else {
				help += "  R obscure"
			}
		}
		bottomLine = lipgloss.NewStyle().Faint(true).Render(help)
	}

	parts := []string{"", statusLine, "", mainArea, ""}
	if nextLine != "" {
		parts = append(parts, nextLine, "")
	}
	parts = append(parts, bottomLine)
	return strings.Join(parts, "\n")
}

// subContext returns 2n+1 subtitle indices centred on the active (or most
// recently ended) subtitle. Slot n is current; 0..n-1 are previous (oldest
// first); n+1..2n are next. -1 means no subtitle for that slot.
func subContext(subs []Subtitle, elapsed time.Duration, n int) []int {
	result := make([]int, 2*n+1)
	for i := range result {
		result[i] = -1
	}
	if len(subs) == 0 {
		return result
	}
	cur := activeSubtitleIdx(subs, elapsed)
	if cur >= 0 {
		result[n] = cur
		for i := 1; i <= n; i++ {
			if cur-i >= 0 {
				result[n-i] = cur - i
			}
			if cur+i < len(subs) {
				result[n+i] = cur + i
			}
		}
		return result
	}
	// In a gap: anchor on the most recently ended subtitle.
	anchor := -1
	for i, s := range subs {
		if s.End <= elapsed {
			anchor = i
		}
	}
	if anchor >= 0 {
		// Prev slots: anchor is the newest past entry (slot n-1).
		for i := 0; i < n; i++ {
			if anchor-i >= 0 {
				result[n-1-i] = anchor - i
			}
		}
		for i := 1; i <= n; i++ {
			if anchor+i < len(subs) {
				result[n+i] = anchor + i
			}
		}
	} else {
		// Before any subtitle: show the first n as upcoming.
		for i := 0; i < n && i < len(subs); i++ {
			result[n+1+i] = i
		}
	}
	return result
}

func (m model) renderDualColumns(w int) string {
	const ctxN = 3
	colW := max((w-3)/2, 10)

	ctx1 := subContext(m.subs, m.elapsed, ctxN)
	ctx2 := subContext(m.subs2, m.elapsed, ctxN)

	var leftParts, rightParts []string

	for i, idx := range ctx1 {
		isCur := i == ctxN
		if idx < 0 {
			leftParts = append(leftParts, "")
			continue
		}
		if isCur {
			leftParts = append(leftParts, m.subs[idx].Text)
		} else {
			t := truncateLine(firstLine(m.subs[idx].Text), colW)
			leftParts = append(leftParts, lipgloss.NewStyle().Faint(true).Render(t))
		}
	}

	for i, idx := range ctx2 {
		isCur := i == ctxN
		isPast := i < ctxN
		if idx < 0 {
			rightParts = append(rightParts, "")
			continue
		}
		if isCur {
			rightParts = append(rightParts, sub2Style.Render(m.sub2Text(idx)))
		} else if isPast {
			t := truncateLine(firstLine(m.subs2[idx].Text), colW)
			rightParts = append(rightParts, lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("220")).Render(t))
		} else {
			// future: follow obscure rules
			t := firstLine(m.subs2[idx].Text)
			if m.obscureMode && !m.revealed[idx] {
				t = obscureText(t)
			}
			t = truncateLine(t, colW)
			rightParts = append(rightParts, lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("220")).Render(t))
		}
	}

	leftBlock := lipgloss.NewStyle().Width(colW).Render(strings.Join(leftParts, "\n"))
	rightBlock := lipgloss.NewStyle().Width(colW).Render(strings.Join(rightParts, "\n"))

	nLines := max(strings.Count(leftBlock, "\n"), strings.Count(rightBlock, "\n")) + 1
	var divLines []string
	for range nLines {
		divLines = append(divLines, " │ ")
	}
	divider := strings.Join(divLines, "\n")

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, divider, rightBlock)
}

// sub2Text returns the text for subs2[idx], obscured if needed.
func (m model) sub2Text(idx int) string {
	if m.obscureMode && !m.revealed[idx] {
		return obscureText(m.subs2[idx].Text)
	}
	return m.subs2[idx].Text
}

// obscureText replaces non-whitespace characters with ░ (light shade block).
func obscureText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\n' {
			b.WriteRune(r)
		} else {
			b.WriteRune('░')
		}
	}
	return b.String()
}

func saveCurrent(m model) {
	curIdx := activeSubtitleIdx(m.subs, m.elapsed)
	cur2Idx := activeSubtitleIdx(m.subs2, m.elapsed)
	if curIdx < 0 && cur2Idx < 0 {
		return
	}

	var lines []string
	if curIdx >= 0 {
		for i, line := range strings.Split(strings.TrimSpace(m.subs[curIdx].Text), "\n") {
			if i == 0 {
				lines = append(lines, "- "+line)
			} else {
				lines = append(lines, "  "+line)
			}
		}
	}
	if cur2Idx >= 0 {
		for _, line := range strings.Split(strings.TrimSpace(m.subs2[cur2Idx].Text), "\n") {
			lines = append(lines, "  - "+line)
		}
	}

	f, err := os.OpenFile(m.mdPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(strings.Join(lines, "\n") + "\n")
}

func doTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func activeSubtitleIdx(subs []Subtitle, elapsed time.Duration) int {
	for i, s := range subs {
		if elapsed >= s.Start && elapsed < s.End {
			return i
		}
	}
	return -1
}

func nextSubtitleIdx(subs []Subtitle, elapsed time.Duration) int {
	for i, s := range subs {
		if s.Start > elapsed {
			return i
		}
	}
	return -1
}

func parseJumpTimestamp(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, true
	}
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
		h, _ := strconv.Atoi(parts[0])
		min, _ := strconv.Atoi(parts[1])
		sec, _ := strconv.ParseFloat(parts[2], 64)
		return time.Duration(h)*time.Hour +
			time.Duration(min)*time.Minute +
			time.Duration(sec*float64(time.Second)), true
	case 2:
		min, _ := strconv.Atoi(parts[0])
		sec, _ := strconv.ParseFloat(parts[1], 64)
		return time.Duration(min)*time.Minute +
			time.Duration(sec*float64(time.Second)), true
	case 1:
		sec, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, false
		}
		return time.Duration(sec * float64(time.Second)), true
	}
	return 0, false
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	tenth := int(d.Milliseconds()/100) % 10
	return fmt.Sprintf("%02d:%02d:%02d.%d", h, m, s, tenth)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func truncateLine(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}

func loadSRT(path string) ([]Subtitle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseSRT(f)
}

func main() {
	sub2Path := flag.String("s2", "", "second subtitle file (displayed in a different colour)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: subplay [options] <file.srt>\n\n")
		fmt.Fprintf(os.Stderr, "options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	subs, err := loadSRT(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", flag.Arg(0), err)
		os.Exit(1)
	}
	if len(subs) == 0 {
		fmt.Fprintf(os.Stderr, "no subtitles found in %s\n", flag.Arg(0))
		os.Exit(1)
	}

	var subs2 []Subtitle
	if *sub2Path != "" {
		subs2, err = loadSRT(*sub2Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading %s: %v\n", *sub2Path, err)
			os.Exit(1)
		}
	}

	sub1Path := flag.Arg(0)
	ext := filepath.Ext(sub1Path)
	mdPath := sub1Path[:len(sub1Path)-len(ext)] + ".md"

	m := model{
		subs:        subs,
		subs2:       subs2,
		obscureMode: len(subs2) > 0,
		revealed:    make(map[int]bool),
		mdPath:      mdPath,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
