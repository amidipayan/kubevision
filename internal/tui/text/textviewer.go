package text

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/amidipayan/kubevision/internal/tui/styles"
)


type CloseTextViewMsg struct{}

type TextViewer struct {
	viewport viewport.Model
	content  string
	title    string
	width    int
	height   int
	ready    bool
}

func NewTextViewer(title, content string, width, height int) *TextViewer {
	
	vpHeight := height - 4
	if vpHeight < 1 {
		vpHeight = 1
	}

	vp := viewport.New(width, vpHeight)
	vp.SetContent(content)
	vp.YPosition = 2 

	return &TextViewer{
		viewport: vp,
		content:  content,
		title:    title,
		width:    width,
		height:   height,
		ready:    true,
	}
}

func (m *TextViewer) Init() tea.Cmd {
	return nil
}

func (m *TextViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "backspace":
			
			return m, func() tea.Msg { return CloseTextViewMsg{} }
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
	}
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *TextViewer) View() string {
	
	header := styles.ResourceTitleStyle.Width(m.width).Render(m.title)

	
	lineCount := strings.Count(m.content, "\n") + 1
	scrollPercent := int(m.viewport.ScrollPercent() * 100)
	
	footerText := fmt.Sprintf(" Lines: %d | Scroll: %d%% | [q/Esc] Close ", lineCount, scrollPercent)
	footer := styles.FooterStyle.Width(m.width).Render(footerText)

	
	return lipgloss.JoinVertical(lipgloss.Left, header, "\n", m.viewport.View(), "\n", footer)
}