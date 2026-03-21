package sre

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/amidipayan/kubevision/internal/k8s/helm"
)


var (
	styleA = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true) 
	styleB = lipgloss.NewStyle().Foreground(lipgloss.Color("#ADFF2F")).Bold(true) 
	styleC = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true) 
	styleD = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true) 
)

type Panel struct {
	Analysis helm.SREAnalysis
	Viewport viewport.Model
	Width    int
	Height   int
	Active   bool
}

func NewPanel(width, height int) *Panel {
	return &Panel{
		Width:    width,
		Height:   height,
		Viewport: viewport.New(width, height-6), 
	}
}

func (p *Panel) Init() tea.Cmd {
	return nil
}

func (p *Panel) SetAnalysis(a helm.SREAnalysis) {
	p.Analysis = a
	p.Viewport.SetContent(p.renderContent())
}

func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	p.Viewport, cmd = p.Viewport.Update(msg)
	return p, cmd
}

func (p *Panel) View() string {
	if !p.Active {
		return ""
	}

	
	gradeStyle := styleD
	switch p.Analysis.SafetyGrade {
	case "A+", "A":
		gradeStyle = styleA
	case "B":
		gradeStyle = styleB
	case "C":
		gradeStyle = styleC
	case "D", "F":
		gradeStyle = styleD
	}

	
	headerText := fmt.Sprintf(" GRADE: %s  |  SCORE: %d  |  RISK: %s ", 
		p.Analysis.SafetyGrade, 
		p.Analysis.Score,
		p.Analysis.PrimaryRiskDriver,
	)
	
	if p.Analysis.PrimaryRiskDriver == "" {
		headerText = fmt.Sprintf(" GRADE: %s  |  SCORE: %d  |  STATUS: Stable ", 
			p.Analysis.SafetyGrade, 
			p.Analysis.Score,
		)
	}

	header := lipgloss.NewStyle().
		Background(gradeStyle.GetForeground()).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Width(p.Width - 4).
		Align(lipgloss.Center).
		Render(headerText)

	
	body := p.Viewport.View()
	
	
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(" [Esc] Close | [↑/↓] Scroll ")

	
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gradeStyle.GetForeground()).
		Padding(0, 1).
		Width(p.Width - 2).
		Height(p.Height - 2)

	return box.Render(lipgloss.JoinVertical(lipgloss.Left, header, "\n", body, "\n", footer))
}

func (p *Panel) renderContent() string {
	var b strings.Builder

	if len(p.Analysis.Results) == 0 {
		return "\n\n   ✨ No heuristics violations found. SRE Approved! 🚀\n"
	}

	
	if len(p.Analysis.RiskFactors) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("⚠  RISK AMPLIFIERS DETECTED:"))
		b.WriteString("\n")
		for _, rf := range p.Analysis.RiskFactors {
			b.WriteString(fmt.Sprintf("   • %s\n", rf))
		}
		b.WriteString("\n")
	}

	wSev := 8
	wCat := 15
	wIssue := 30

	wSymptom := p.Width - wSev - wCat - wIssue - 6
	if wSymptom < 10 { wSymptom = 10 } 

	
	renderCell := func(text string, width int, style lipgloss.Style) string {
		return style.Width(width).MaxWidth(width).Render(text)
	}

	
	renderWrapCell := func(text string, width int, style lipgloss.Style) string {
		return style.Width(width).Render(text) 
	}

	
	styleHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	styleSev := lipgloss.NewStyle().Bold(true)
	styleCat := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	styleIssue := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	styleSymptom := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))

	
	headerRow := lipgloss.JoinHorizontal(lipgloss.Left,
		renderCell("SEV", wSev, styleHeader),
		renderCell("CATEGORY", wCat, styleHeader),
		renderCell("ISSUE", wIssue, styleHeader),
		renderCell("SYMPTOM", wSymptom, styleHeader),
	)
	
	b.WriteString(headerRow + "\n")
	b.WriteString(strings.Repeat("─", p.Width-6) + "\n")

	for _, r := range p.Analysis.Results {
		
		sevColor := "#00AAFF" 
		if r.Severity == helm.SevMedium { sevColor = "#FFA500" } 
		if r.Severity == helm.SevHigh { sevColor = "#FF0000" }   
		if r.Severity == helm.SevCritical { sevColor = "#FF0055" } 

		
		cSev := renderCell(string(r.Severity), wSev, styleSev.Foreground(lipgloss.Color(sevColor)))
		cCat := renderCell(string(r.Category), wCat, styleCat)
		
		
		cIssue := renderCell(truncate(r.Title, wIssue-2), wIssue, styleIssue)
		
		
		cSymptom := renderWrapCell(r.Symptom, wSymptom, styleSymptom)

		
		row := lipgloss.JoinHorizontal(lipgloss.Top, cSev, cCat, cIssue, cSymptom)
		b.WriteString(row + "\n")


		desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("   ↳ %s", r.Description))
		
		infoStr := fmt.Sprintf(" [-%0.1f: %s]", r.ScoreImpact, r.Calculation)
		math := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Italic(true).Render(infoStr)
		
		b.WriteString(fmt.Sprintf("%s %s\n", desc, math))

		fix := lipgloss.NewStyle().Foreground(lipgloss.Color("#00AA00")).Render(fmt.Sprintf("   🔧 Fix: %s", r.Remediation))
		b.WriteString(fix + "\n\n")
	}

	return b.String()
}

func truncate(s string, l int) string {
	if len(s) > l {
		return s[:l-1] + "…"
	}
	return s
}