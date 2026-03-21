package logs

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/amidipayan/kubevision/internal/utils"
)


type ReloadLogMsg struct {
	Duration time.Duration
}


type CloseLogViewMsg struct{}


type logChunkMsg struct {
	id   int64
	data []byte
}


type errMsg struct{ err error }


type clearStatusMsg struct{}



type LogViewer struct {
	id int64 

	
	title   string
	podName string

	
	content    []string 
	timestamps []string 
	markedLine int      

	
	width      int
	height     int
	autoScroll bool
	showTime   bool
	timeLabel  string        
	stream     io.ReadCloser 
	err        error

	
	statusMsg string


	searchMode   bool
	textInput    textinput.Model
	matches      []int
	matchIndex   int
	totalMatches int
	searchTerm   string

	
	viewport viewport.Model

	
	styleHeader    lipgloss.Style
	styleMatch     lipgloss.Style
	styleMatchCurr lipgloss.Style
	styleFlash     lipgloss.Style 
	styleLineNum   lipgloss.Style 
}


func NewLogViewer(title, podName string, width, height int, stream io.ReadCloser) *LogViewer {

	safeHeight := height - 7
	if safeHeight < 1 {
		safeHeight = 1
	}

	vp := viewport.New(width, safeHeight)
	vp.YPosition = 0

	ti := textinput.New()
	ti.Placeholder = "Search logs... (Use 'regex:' prefix)"
	ti.Prompt = "/"
	ti.CharLimit = 200
	ti.Width = 50

	return &LogViewer{
		id:         time.Now().UnixNano(), 
		title:      title,
		podName:    podName,
		width:      width,
		height:     height,
		stream:     stream,
		viewport:   vp,
		textInput:  ti,
		autoScroll: true,
		showTime:   true,
		markedLine: -1,
		timeLabel:  "All",

		content:    make([]string, 0),
		timestamps: make([]string, 0),
		matches:    make([]int, 0),

		
		styleHeader:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#553388")).Padding(0, 1),
		styleMatch:     lipgloss.NewStyle().Background(lipgloss.Color("#444400")).Foreground(lipgloss.Color("#FFFF00")),
		styleMatchCurr: lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00")).Foreground(lipgloss.Color("#000000")),
		
		
		styleFlash:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")).Background(lipgloss.Color("#000000")).Bold(true).Padding(0, 1),
		
		
		styleLineNum:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Bold(true), 
	}
}


func (m *LogViewer) Close() {
	if m.stream != nil {
		m.stream.Close()
	}
}

func (m *LogViewer) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.readNextChunk())
}

func (m *LogViewer) SetTimeFilter(label string) {
	m.timeLabel = label
}

func (m *LogViewer) readNextChunk() tea.Cmd {

	currentID := m.id
	
	return func() tea.Msg {
		if m.stream == nil {
			return nil
		}
		
		buf := make([]byte, 4096)
		n, err := m.stream.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return errMsg{err}
		}
		
		return logChunkMsg{id: currentID, data: buf[:n]}
	}
}



func (m *LogViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		
		safeHeight := msg.Height - 7
		if safeHeight < 1 { safeHeight = 1 }
		m.viewport.Width = msg.Width
		m.viewport.Height = safeHeight
		m.refreshViewport()

	case logChunkMsg:
	
		if msg.id != m.id {
			return m, nil
		}

		raw := string(msg.data)
		newLines := strings.Split(raw, "\n")
		for _, line := range newLines {
			if line == "" { continue }
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				m.timestamps = append(m.timestamps, parts[0])
				m.content = append(m.content, parts[1])
			} else {
				m.timestamps = append(m.timestamps, "")
				m.content = append(m.content, line)
			}
		}

		if m.searchTerm != "" {
			m.updateSearchForNewLines(len(m.content) - len(newLines))
		}
		
		m.refreshViewport()

		
		if m.autoScroll {
			m.viewport.GotoBottom()
		}
		cmds = append(cmds, m.readNextChunk())

	case clearStatusMsg:
		m.statusMsg = ""

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
	
		if m.searchMode {
			switch msg.Type {
			case tea.KeyEnter:
				m.searchMode = false
				m.searchTerm = m.textInput.Value()
				m.performSearch()
				m.textInput.Blur()
			case tea.KeyEsc:
				m.searchMode = false
				m.textInput.Blur()
			}
			m.textInput, cmd = m.textInput.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

	
		switch msg.String() {
		case "q", "esc":
			m.Close() 
			return m, func() tea.Msg { return CloseLogViewMsg{} }

		
		case "up", "k":
			m.autoScroll = false; m.viewport.LineUp(1)
		case "down", "j":
			m.autoScroll = false; m.viewport.LineDown(1)
		case "pgup":
			m.autoScroll = false; m.viewport.ViewUp()
		case "pgdown":
			m.autoScroll = false; m.viewport.ViewDown()
		case "home":
			m.autoScroll = false; m.viewport.GotoTop()
		
		case "G": 
			m.autoScroll = true; m.viewport.GotoBottom(); m.markedLine = -1
			m.triggerFlash("RESUMED TAIL")
		
		case "g":
			m.autoScroll = false; m.viewport.GotoTop()

		case "/":
			m.searchMode = true; m.textInput.Focus(); return m, textinput.Blink
		case "n":
			if m.totalMatches > 0 {
				m.matchIndex = (m.matchIndex + 1) % m.totalMatches
				m.scrollToMatch()
			}
		case "N":
			if m.totalMatches > 0 {
				m.matchIndex--
				if m.matchIndex < 0 { m.matchIndex = m.totalMatches - 1 }
				m.scrollToMatch()
			}

		case "t":
			m.showTime = !m.showTime; m.refreshViewport()

		case "C":
			m.content = nil; m.timestamps = nil; m.matches = nil; m.totalMatches = 0
			m.viewport.SetContent(""); m.markedLine = -1
			m.triggerFlash("LOGS CLEARED")

		case "c":
			txt := m.viewport.View()
			if m.markedLine != -1 && m.markedLine < len(m.content) { 
				txt = m.content[m.markedLine] 
			}
			utils.CopyToClipboard(txt)
			m.triggerFlash("COPIED TO CLIPBOARD")

		case "s":
			go func() {
				f := fmt.Sprintf("%s_logs_%d.txt", m.podName, time.Now().Unix())
				os.WriteFile(f, []byte(strings.Join(m.content, "\n")), 0644)
			}()
			m.triggerFlash("LOGS SAVED TO FILE")

		
		case "m":
			m.autoScroll = false
			
			
			var targetLine int

			if len(m.content) == 0 {
				return m, nil
			}

			
			if m.viewport.AtBottom() {
				targetLine = len(m.content) - 1
			} else {
				
				targetLine = m.viewport.YOffset
			}
			
			
			if targetLine >= len(m.content) {
				targetLine = len(m.content) - 1
			}
			if targetLine < 0 { targetLine = 0 }
			
			m.markedLine = targetLine
			
			
			m.viewport.SetYOffset(targetLine)
			m.refreshViewport()
			
			m.triggerFlash(fmt.Sprintf("MARKED LINE %d", m.markedLine+1))

		}

		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *LogViewer) triggerFlash(msg string) {
	m.statusMsg = msg
	
	go func() {
		time.Sleep(2 * time.Second)
	}()
}



func (m *LogViewer) performSearch() {
	if m.searchTerm == "" {
		m.matches = nil; m.totalMatches = 0; m.refreshViewport(); return
	}
	m.matches = nil
	isRegex := strings.HasPrefix(m.searchTerm, "regex:")
	cleanTerm := strings.TrimPrefix(m.searchTerm, "regex:")
	var rx *regexp.Regexp
	if isRegex { rx, _ = regexp.Compile(cleanTerm) }

	for i, line := range m.content {
		match := false
		if isRegex && rx != nil { match = rx.MatchString(line)
		} else if !isRegex { match = strings.Contains(strings.ToLower(line), strings.ToLower(cleanTerm)) }
		if match { m.matches = append(m.matches, i) }
	}
	m.totalMatches = len(m.matches); m.matchIndex = 0
	if m.totalMatches > 0 { m.autoScroll = false; m.scrollToMatch() }
	m.refreshViewport()
}

func (m *LogViewer) updateSearchForNewLines(startIndex int) {
	if startIndex < 0 { startIndex = 0 }
	isRegex := strings.HasPrefix(m.searchTerm, "regex:")
	cleanTerm := strings.TrimPrefix(m.searchTerm, "regex:")
	var rx *regexp.Regexp
	if isRegex { rx, _ = regexp.Compile(cleanTerm) }

	for i := startIndex; i < len(m.content); i++ {
		line := m.content[i]
		match := false
		if isRegex && rx != nil { match = rx.MatchString(line)
		} else if !isRegex { match = strings.Contains(strings.ToLower(line), strings.ToLower(cleanTerm)) }
		if match { m.matches = append(m.matches, i) }
	}
	m.totalMatches = len(m.matches)
}

func (m *LogViewer) scrollToMatch() {
	if len(m.matches) > 0 {
		m.viewport.SetYOffset(m.matches[m.matchIndex])
		m.refreshViewport()
	}
}

func (m *LogViewer) refreshViewport() {
	var b strings.Builder
	styleMatch := lipgloss.NewStyle().Background(lipgloss.Color("#444400")).Foreground(lipgloss.Color("#FFFF00"))
	styleMatchCurr := lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00")).Foreground(lipgloss.Color("#000000"))
	styleMark := lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Bold(true)
	styleTime := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	matchMap := make(map[int]bool)
	for _, idx := range m.matches { matchMap[idx] = true }

	for i, line := range m.content {
		
		lineNum := m.styleLineNum.Render(fmt.Sprintf("%4d ", i+1))

		
		prefix := ""
		if m.showTime && i < len(m.timestamps) {
			prefix = styleTime.Render(m.timestamps[i]) + " "
		}

		
		row := fmt.Sprintf("%s%s%s", lineNum, prefix, line)
		
		
		if matchMap[i] {
			if i == m.matches[m.matchIndex] { row = styleMatchCurr.Render(row)
			} else { row = styleMatch.Render(row) }
		}

		
		if i == m.markedLine { row = styleMark.Render(fmt.Sprintf("> %s", row)) }
		
		b.WriteString(row + "\n")
	}
	m.viewport.SetContent(b.String())
}



func (m *LogViewer) View() string {
	
	status := "STREAMING"
	statusColor := "#00FF00" 
	if !m.autoScroll {
		status = "PAUSED (Press G to Resume)" 
		statusColor = "#FF0000" 
	}
	statusBlock := lipgloss.NewStyle().Background(lipgloss.Color(statusColor)).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1).Render(status)
	
	
	var infoBlock string
	if m.statusMsg != "" {
		infoBlock = m.styleFlash.Render(fmt.Sprintf(" %s ", m.statusMsg))
	} else {
		infoText := fmt.Sprintf("Filter: %s", m.timeLabel)
		if m.searchTerm != "" { infoText += fmt.Sprintf(" | Match: %d/%d", m.matchIndex+1, m.totalMatches) }
		infoBlock = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Foreground(lipgloss.Color("#00FFFF")).Padding(0, 1).Render(infoText)
	}

	
	markInfo := ""
	if m.markedLine != -1 { 
		markInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Render(fmt.Sprintf(" [Marked: L%d]", m.markedLine+1)) 
	}

	
	headerLeft := m.styleHeader.Render(fmt.Sprintf(" %s ", m.title))
	
	var header string
	if m.searchMode {
		header = lipgloss.JoinHorizontal(lipgloss.Left, headerLeft, lipgloss.NewStyle().Width(2).Render(" "), m.textInput.View())
	} else {
		gapWidth := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(statusBlock) - lipgloss.Width(infoBlock) - lipgloss.Width(markInfo)
		if gapWidth < 0 { gapWidth = 0 }
		
		header = lipgloss.JoinHorizontal(lipgloss.Top, headerLeft, markInfo, strings.Repeat(" ", gapWidth), infoBlock, statusBlock)
	}

	
	navHelp := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render("NAV: g/G Top/Bot j/k Scroll")
	viewHelp := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("VIEW: Time t Stamp")
	actHelp := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("ACT: / Find m Mark c Copy s Save C Clear")

	footer := lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("#1A1A1A")).
		Padding(0, 1).
		Align(lipgloss.Left).
		Render(navHelp + " | " + viewHelp + " | " + actHelp)

	return lipgloss.JoinVertical(lipgloss.Left, header, "\n", m.viewport.View(), "\n", footer)
}