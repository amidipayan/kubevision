package explorer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/k8s/helm"
	"github.com/amidipayan/kubevision/internal/tui/styles"
	"github.com/amidipayan/kubevision/internal/utils"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)


const (
	TabOverview = iota
	TabValues
	TabHistory
	TabDiff
	TabDrift
	TabSecurity
)


var DashboardTabs = []string{"Overview", "Values", "History", "Diff", "Drift", "Security"}


type tickMsg time.Time

type DashResourceSelectMsg struct {
	Resource helm.ManagedResource
}

type driftDiffLoadedMsg struct {
	content string
	desired string 
	live    string 
	err     error
}

type ManagedResource = helm.ManagedResource

type AggregatedEvent struct {
	Reason   string
	Message  string
	Object   string
	Count    int
	Type     string
	LastTime time.Time
}

type HelmDashboard struct {
	client      *client.KubeClient
	analyzer    *helm.Analyzer
	releaseName string
	namespace   string

	activeTab int
	width     int
	height    int

	viewport      viewport.Model
	resViewport   viewport.Model
	evtViewport   viewport.Model
	driftViewport viewport.Model

	release     *release.Release
	prevRelease *release.Release
	history     []*release.Release
	
	valuesAll   map[string]interface{}
	valuesUser  map[string]interface{}
	userKeys    map[string]bool

	liveEvents  []corev1.Event

	
	diffBase   *release.Release
	diffTarget *release.Release
	diffMode   string

	analysis *helm.AnalysisResult

	
	driftedResources []ManagedResource
	driftCursor      int
	driftDiffContent string
	driftDesiredYAML string 
	driftLiveYAML    string 
	driftLoading     bool
	driftFullscreen  bool   

	verdictIcon    string
	verdictColor   lipgloss.Color
	statusSentence string

	healthGrade string

	riskScore     string
	blastRadius   string
	securityCount int

	driftStatus  string

	aggregatedEvents []AggregatedEvent

	inventoryItems  []ManagedResource
	inventoryCursor int

	
	historyCursor int

	age         string
	lastUpdated time.Time

	valuesFilter string
	inputMode    bool

	
	securityCursor     int
	securityDetailMode bool               
	securityFindings   []helm.CheckResult 

	loading  bool
	fetching bool 
	err      error
}

func NewHelmDashboard(c *client.KubeClient, ns, name string, width, height int) *HelmDashboard {
	vp := viewport.New(width, height)
	rvp := viewport.New(width, height)
	evp := viewport.New(width, height)
	dvp := viewport.New(width, height) 

	return &HelmDashboard{
		client:        c,
		analyzer:      helm.NewAnalyzer(c),
		releaseName:   name,
		namespace:     ns,
		width:         width,
		height:        height,
		viewport:      vp,
		resViewport:   rvp,
		evtViewport:   evp,
		driftViewport: dvp,
		loading:       true,
		diffMode:      "auto",
	}
}

func (h *HelmDashboard) Init() tea.Cmd {
	return tea.Batch(h.fetchData(), h.tick())
}

func (h *HelmDashboard) tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}


func (h *HelmDashboard) fetchData() tea.Cmd {
	return func() tea.Msg {
		cfg, err := h.client.NewHelmConfiguration(h.namespace)
		if err != nil {
			return dashboardErrorMsg{err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		
		rel, err := action.NewGet(cfg).Run(h.releaseName)
		if err != nil {
			return dashboardErrorMsg{err}
		}

		
		histClient := action.NewHistory(cfg)
		histClient.Max = 10
		hist, _ := histClient.Run(h.releaseName)

		sort.Slice(hist, func(i, j int) bool {
			return hist[i].Version > hist[j].Version
		})

		var prevRel *release.Release
		if len(hist) > 1 && rel.Version > 1 {
			for _, r := range hist {
				if r.Version == rel.Version-1 {
					prevRel = r
					break
				}
			}
		}

		
		valClient := action.NewGetValues(cfg)
		valClient.AllValues = true
		valsAll, _ := valClient.Run(h.releaseName)

		valClientUser := action.NewGetValues(cfg)
		valClientUser.AllValues = false
		valsUser, _ := valClientUser.Run(h.releaseName)

		
		events, _ := h.client.GetClientset().CoreV1().Events(h.namespace).List(ctx, metav1.ListOptions{})

		
		analysis, err := h.analyzer.AnalyzeRelease(rel, hist)
		if err != nil {
			return dashboardErrorMsg{err}
		}

		return dashboardLoadedMsg{
			release:     rel,
			prevRelease: prevRel,
			history:     hist,
			valuesAll:   valsAll,
			valuesUser:  valsUser,
			events:      events.Items,
			analysis:    analysis,
		}
	}
}

type dashboardLoadedMsg struct {
	release     *release.Release
	prevRelease *release.Release
	history     []*release.Release
	valuesAll   map[string]interface{}
	valuesUser  map[string]interface{}
	events      []corev1.Event
	analysis    *helm.AnalysisResult
}

type dashboardErrorMsg struct{ err error }

func (h *HelmDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height
		
		
		safeHeight := msg.Height - 3
		if safeHeight < 1 { safeHeight = 1 }
		h.viewport.Width = msg.Width
		h.viewport.Height = safeHeight
		h.driftViewport.Width = msg.Width
		h.driftViewport.Height = safeHeight
		
		if h.activeTab == TabDrift && h.driftDiffContent != "" {
			h.refreshDriftDiff()
		}

	case tickMsg:
		if !h.loading && !h.fetching {
			h.fetching = true
			cmds = append(cmds, h.fetchData())
		}
		cmds = append(cmds, h.tick())

	case driftDiffLoadedMsg:
		h.driftLoading = false
		if msg.err != nil {
			h.driftDiffContent = fmt.Sprintf("Error generating diff: %v", msg.err)
		} else {
			h.driftDesiredYAML = msg.desired
			h.driftLiveYAML = msg.live
			h.driftFullscreen = true
			h.refreshDriftDiff()
		}
		h.driftViewport.GotoTop()

	case dashboardLoadedMsg:
		h.loading = false
		h.fetching = false
		h.lastUpdated = time.Now()
		
		h.release = msg.release
		h.prevRelease = msg.prevRelease
		h.history = msg.history
		h.valuesAll = msg.valuesAll
		h.valuesUser = msg.valuesUser
		h.liveEvents = msg.events
		h.analysis = msg.analysis

		h.userKeys = make(map[string]bool)
		flattenMap("", h.valuesUser, h.userKeys)

		if h.diffMode == "auto" {
			h.diffTarget = h.release
			h.diffBase = h.prevRelease
		}

		h.calculateTimings()
		h.computeHealthAndDrift()
		h.processEvents(msg.events)
		h.synthesizeOverallHealth()

		
		if h.analysis != nil {
			h.securityFindings = h.analysis.SecurityProfile.Findings
		}

	case dashboardErrorMsg:
		h.loading = false
		h.fetching = false
		h.err = msg.err

	case tea.KeyMsg:
		if h.inputMode {
			switch msg.Type {
			case tea.KeyEnter, tea.KeyEsc:
				h.inputMode = false
				return h, nil
			case tea.KeyBackspace:
				if len(h.valuesFilter) > 0 {
					h.valuesFilter = h.valuesFilter[:len(h.valuesFilter)-1]
				}
			case tea.KeyRunes:
				h.valuesFilter += string(msg.Runes)
			}
			return h, nil
		}

		switch msg.String() {
		case "enter":
			if h.activeTab == TabOverview && len(h.inventoryItems) > 0 {
				selected := h.inventoryItems[h.inventoryCursor]
				return h, func() tea.Msg { return DashResourceSelectMsg{Resource: selected} }
			}
			if h.activeTab == TabHistory && len(h.history) > 0 {
				selectedRev := h.history[h.historyCursor]
				h.diffBase = selectedRev
				h.diffTarget = h.release
				h.diffMode = "manual"
				h.activeTab = TabDiff
				return h, nil
			}
			if h.activeTab == TabDrift && len(h.driftedResources) > 0 {
				selected := h.driftedResources[h.driftCursor]
				h.driftLoading = true
				h.driftDiffContent = ""
				return h, h.calculateDriftDiff(selected)
			}
			
			if h.activeTab == TabSecurity && len(h.securityFindings) > 0 {
				h.securityDetailMode = !h.securityDetailMode
				h.updateViewport() 
				return h, nil
			}

		case "f":
			if h.activeTab == TabDrift && h.driftDiffContent != "" {
				h.driftFullscreen = !h.driftFullscreen
				h.refreshDriftDiff()
				return h, nil
			}

		case "1", "i": h.activeTab = TabOverview
		case "2", "v": h.activeTab = TabValues
		case "3", "H": h.activeTab = TabHistory
		case "4", "C": h.activeTab = TabDiff
		case "5", "W": h.activeTab = TabDrift
		case "6", "K": h.activeTab = TabSecurity

		case "/":
			if h.activeTab == TabValues {
				h.inputMode = true
				h.valuesFilter = ""
			}

		case "y":
			cmds = append(cmds, h.copyCLICommand())

		case "tab", "l", "right":
			h.activeTab++
			if h.activeTab > TabSecurity { h.activeTab = TabOverview }
		case "shift+tab", "left":
			h.activeTab--
			if h.activeTab < 0 { h.activeTab = TabSecurity }
		
		
		case "up", "k", "down", "j", "pgup", "pgdown":
			
			if h.activeTab == TabOverview && len(h.inventoryItems) > 0 {
				if msg.String() == "up" || msg.String() == "k" { h.inventoryCursor = clamp(h.inventoryCursor-1, 0, len(h.inventoryItems)-1) }
				if msg.String() == "down" || msg.String() == "j" { h.inventoryCursor = clamp(h.inventoryCursor+1, 0, len(h.inventoryItems)-1) }
			} else if h.activeTab == TabHistory && len(h.history) > 0 {
				if msg.String() == "up" || msg.String() == "k" { h.historyCursor = clamp(h.historyCursor-1, 0, len(h.history)-1) }
				if msg.String() == "down" || msg.String() == "j" { h.historyCursor = clamp(h.historyCursor+1, 0, len(h.history)-1) }
			} else if h.activeTab == TabDrift && !h.driftFullscreen && len(h.driftedResources) > 0 {
				if msg.String() == "up" || msg.String() == "k" { h.driftCursor = clamp(h.driftCursor-1, 0, len(h.driftedResources)-1) }
				if msg.String() == "down" || msg.String() == "j" { h.driftCursor = clamp(h.driftCursor+1, 0, len(h.driftedResources)-1) }
			} else if h.activeTab == TabSecurity && !h.securityDetailMode && len(h.securityFindings) > 0 {
				
				if msg.String() == "up" || msg.String() == "k" { 
					h.securityCursor = clamp(h.securityCursor-1, 0, len(h.securityFindings)-1) 
				}
				if msg.String() == "down" || msg.String() == "j" { 
					h.securityCursor = clamp(h.securityCursor+1, 0, len(h.securityFindings)-1) 
				}
				h.updateViewport() 
				return h, nil
			}

			
			var cmd tea.Cmd
			if h.activeTab == TabDrift {
				h.driftViewport, cmd = h.driftViewport.Update(msg)
			} else {
				h.viewport, cmd = h.viewport.Update(msg)
			}
			return h, cmd
		}

		if h.activeTab != TabOverview && h.activeTab != TabSecurity {
			var cmd tea.Cmd
			if h.activeTab == TabDrift {
				h.driftViewport, cmd = h.driftViewport.Update(msg)
			} else {
				h.viewport, cmd = h.viewport.Update(msg)
			}
			cmds = append(cmds, cmd)
		}
	}

	return h, tea.Batch(cmds...)
}

func (h *HelmDashboard) View() string {
	if h.loading && h.release == nil {
		return "\n  ⏳ Analyzing Helm Release... Loading Intelligence..."
	}
	if h.err != nil {
		return fmt.Sprintf("\n  ❌ Error: %v", h.err)
	}

	
	var renderedTabs []string
	totalTabWidth := 0
	for i, t := range DashboardTabs {
		var style lipgloss.Style
		if i == h.activeTab {
			style = styles.TabActiveStyle
		} else {
			style = styles.TabInactiveStyle
		}
		
		tabContent := fmt.Sprintf("[%d] %s", i+1, t)
		rendered := style.Render(tabContent)
		totalTabWidth += lipgloss.Width(rendered)
		renderedTabs = append(renderedTabs, rendered)
	}
	gapWidth := h.width - totalTabWidth
	if gapWidth < 0 { gapWidth = 0 }
	gap := styles.TabGapStyle.Width(gapWidth).Render("")
	
	tabHeader := lipgloss.JoinHorizontal(lipgloss.Top, append(renderedTabs, gap)...)
	
	
	headerHeight := lipgloss.Height(tabHeader)

	
	var content string
	

	availableHeight := h.height - headerHeight
	if availableHeight < 0 { availableHeight = 0 }
	
	h.viewport.Width = h.width
	h.viewport.Height = availableHeight

	switch h.activeTab {
	case TabOverview:
		
		content = h.renderOverviewLayout(headerHeight)
	case TabValues:
		h.viewport.SetContent(h.renderValues())
		content = h.viewport.View()
	case TabHistory:
		h.viewport.SetContent(h.renderHistory())
		content = h.viewport.View()
	case TabDiff:
		h.viewport.SetContent(h.renderDiff())
		content = h.viewport.View()
	case TabDrift:
		content = h.renderDrift() 
	case TabSecurity:
		h.viewport.SetContent(h.renderSecurity())
		content = h.viewport.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabHeader, content)
}

func (h *HelmDashboard) updateViewport() {
	var content string
	switch h.activeTab {
	case TabSecurity:
		content = h.renderSecurity()
	}
	h.viewport.SetContent(content)
}


func (h *HelmDashboard) renderSecurity() string {
	if h.analysis == nil {
		return "Running Security Analysis..."
	}
	
	var b strings.Builder
	profile := h.analysis.SecurityProfile


	gradeColor := "#00FF00" // A
	if strings.HasPrefix(profile.Grade, "B") { gradeColor = "#ADFF2F" }
	if strings.HasPrefix(profile.Grade, "C") { gradeColor = "#FFA500" }
	if strings.HasPrefix(profile.Grade, "D") { gradeColor = "#FF5555" }
	if strings.HasPrefix(profile.Grade, "F") { gradeColor = "#FF0000" }

	headerStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Align(lipgloss.Center)
	
	
	gradeBox := headerStyle.BorderForeground(lipgloss.Color(gradeColor)).Render(
		fmt.Sprintf("%s\n%d/100", 
			lipgloss.NewStyle().Foreground(lipgloss.Color(gradeColor)).Bold(true).Render("GRADE "+profile.Grade),
			profile.Score),
	)

	
	velocityColor := "#00FF00"
	velocityIcon := "→"
	if profile.RiskVelocity == "Increasing" {
		velocityColor = "#FF5555"
		velocityIcon = "↗"
	}
	velocityBox := headerStyle.Render(
		fmt.Sprintf("Risk Velocity\n%s %s", 
			lipgloss.NewStyle().Foreground(lipgloss.Color(velocityColor)).Bold(true).Render(velocityIcon),
			profile.RiskVelocity),
	)

	
	blast := profile.BlastRadiusInfo
	blastScoreColor := "#00FF00"
	if blast.Score > 2.0 { blastScoreColor = "#FFA500" }
	if blast.Score > 4.0 { blastScoreColor = "#FF0000" }
	
	blastBox := headerStyle.Render(
		fmt.Sprintf("Blast Radius\n%s / 5.0", 
			lipgloss.NewStyle().Foreground(lipgloss.Color(blastScoreColor)).Bold(true).Render(fmt.Sprintf("%0.1f", blast.Score))),
	)

	
	nextActionMsg := "✅ All Clear"
	nextActionEffort := ""
	if profile.NextBestAction != nil {
		nextActionMsg = profile.NextBestAction.Message
		nextActionEffort = fmt.Sprintf("(%s)", profile.NextBestAction.Effort)
	}
	
	actionWidth := h.width - lipgloss.Width(gradeBox) - lipgloss.Width(velocityBox) - lipgloss.Width(blastBox) - 10
	if actionWidth < 20 { actionWidth = 20 }

	actionBox := headerStyle.Width(actionWidth).BorderForeground(lipgloss.Color("#00FFFF")).Render(
		fmt.Sprintf("%s\n%s %s", 
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Bold(true).Render("NEXT BEST ACTION"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render("➜ "+nextActionMsg),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Render(nextActionEffort)),
	)

	headerRow := lipgloss.JoinHorizontal(lipgloss.Top, gradeBox, " ", velocityBox, " ", blastBox, " ", actionBox)
	b.WriteString(lipgloss.NewStyle().Padding(0, 1).Render(headerRow) + "\n")

	
	legend := fmt.Sprintf("%s | %s | %s | %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("CRIT 🔴"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("HIGH 🟠"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("MED 🟡"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("LOW 🟢"),
	)
	b.WriteString(lipgloss.NewStyle().Align(lipgloss.Right).Width(h.width-4).Render(legend) + "\n")

	
	if h.securityDetailMode && len(h.securityFindings) > 0 {
		
		sel := h.securityFindings[h.securityCursor]
		
		b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("#005F87")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1).Width(h.width).Render(" ▲ FINDING DETAIL (Press <Enter> to Back) ") + "\n\n")
		
		
		b.WriteString(fmt.Sprintf("Issue:      %s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(sel.Message)))
		
		
		meta := fmt.Sprintf("Severity:   %s   Confidence: %s   Effort: %s", 
			lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorError)).Render(sel.Severity), 
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render(sel.Confidence),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(sel.Effort),
		)
		b.WriteString(meta + "\n\n")

		
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render("Why this matters:") + "\n")
		b.WriteString(fmt.Sprintf("  %s\n\n", sel.Why))

		
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render("How to fix:") + "\n")
		b.WriteString(fmt.Sprintf("  %s\n", sel.Remediation))

		
		if sel.RemediationCode != "" {
			b.WriteString("\n")
			codeBlock := lipgloss.NewStyle().
				Background(lipgloss.Color("#222222")).
				Foreground(lipgloss.Color("#CCCCCC")).
				Padding(1, 2).
				Width(h.width - 10).
				Render(sel.RemediationCode)
			b.WriteString(codeBlock)
		}

	} else {
		
		b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1).Width(h.width).Render(" ▼ TOP RISKS (Select & Press <Enter>) ") + "\n")
		
		if len(h.securityFindings) == 0 {
			b.WriteString("\n   ✨ No security risks detected. Excellent work!")
		} else {
			
			for i, f := range h.securityFindings {
				cursor := "  "
				bg := lipgloss.Color("")
				fg := lipgloss.Color("#CCCCCC")
				
				if i == h.securityCursor {
					cursor = "▶ "
					bg = lipgloss.Color("#444444")
					fg = lipgloss.Color("#FFFFFF")
				}
				
				
				sevColor := "#00FF00"
				if f.Severity == "CRITICAL" { sevColor = "#FF0000" }
				if f.Severity == "HIGH" { sevColor = "#FF5555" }
				if f.Severity == "MED" { sevColor = "#FFA500" }

				
				line := fmt.Sprintf("%s[%s][%s] %s", 
					cursor,
					lipgloss.NewStyle().Foreground(lipgloss.Color(sevColor)).Render(f.Severity[:1]), 
					lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render(f.Effort),
					f.Message,
				)
				
				b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(fg).Width(h.width).Render(line) + "\n")
			}
		}

		
		b.WriteString("\n" + lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1).Width(h.width).Render(" ▼ BLAST RADIUS CONTEXT ") + "\n")
		b.WriteString(fmt.Sprintf("   • Namespace: %s\n   • Network:   %s\n   • RBAC:      %s\n   • Privilege: %s\n", 
			blast.NamespaceLevel, blast.NetworkLevel, blast.RBACLevel, blast.PrivilegeLevel))
	}

	return b.String()
}

func (h *HelmDashboard) renderOverviewLayout(headerHeight int) string {
	availableHeight := h.height - headerHeight - 1
	if availableHeight < 1 { availableHeight = 1 }

	boxWidth := h.width - 2


	detailsH := 9
	if availableHeight < 20 { detailsH = 7 }
	if availableHeight < 15 { detailsH = 5 } 
	
	
	if detailsH > availableHeight {
		detailsH = availableHeight
	}
	
	details := h.renderDetailsBox(boxWidth, detailsH)
	
	remaining := availableHeight - detailsH
	if remaining <= 0 {
		return details
	}


	resH := int(float64(remaining) * 0.6)
	if resH < 5 { resH = 5 } 
	
	
	eventsH := remaining - resH
	

	if eventsH < 3 {
		resH = remaining 
		eventsH = 0
	}
	
	
	if resH > remaining {
		resH = remaining
		eventsH = 0
	}

	resources := h.renderResourcesBox(boxWidth, resH)
	
	if eventsH > 0 {
		events := h.renderEventsBox(boxWidth, eventsH)
		return lipgloss.JoinVertical(lipgloss.Left, details, resources, events)
	}

	return lipgloss.JoinVertical(lipgloss.Left, details, resources)
}

func (h *HelmDashboard) renderDetailsBox(w, height int) string {
	
	if height < 2 { return "" }
	
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Width(w).Height(height - 2)
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Bold(true).Render(" DETAILS & RISK PROFILE ")

	
	if height <= 5 {
		status := fmt.Sprintf("%s %s", h.verdictIcon, h.statusSentence)
		return border.Render(lipgloss.JoinVertical(lipgloss.Left, title, status))
	}

	leftWidth := w / 2
	rightWidth := w - leftWidth - 4

	lContent := fmt.Sprintf("Namespace: %s\nChart:     %s\nVersion:   %s\nDeployed:  %s", h.namespace, h.release.Chart.Metadata.Name, h.release.Chart.Metadata.Version, h.age)

	drift := "SYNCED"
	if h.analysis != nil {
		drift = h.analysis.DriftStatus
	}

	rContent := fmt.Sprintf("Risk Score:   %s\nBlast Radius: %s\nSecurity:     %d issues\nDrift:        %s",
		h.riskScore, h.blastRadius, h.securityCount, drift)

	content := lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(leftWidth).Render(lContent), lipgloss.NewStyle().Width(rightWidth).Render(rContent))
	
	pulse := "⚡"
	if h.fetching { pulse = "⏳" }
	
	verdict := lipgloss.NewStyle().Foreground(h.verdictColor).Bold(true).Render(fmt.Sprintf("%s %s: %s", h.verdictIcon, h.healthGrade, h.statusSentence))
	
	
	if height >= 9 {
		lastUpd := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("Last Update: %s %s", h.lastUpdated.Format("15:04:05"), pulse))
		return border.Render(lipgloss.JoinVertical(lipgloss.Left, title, verdict, content, lastUpd))
	}

	return border.Render(lipgloss.JoinVertical(lipgloss.Left, title, verdict, content))
}

func (h *HelmDashboard) renderResourcesBox(w, height int) string {
	
	if height < 4 { return "" }
	
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Width(w).Height(height)

	colCursor := 4
	colVisual := 8
	colKind := 45
	colName := 60
	colStatus := 25

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	headerRow := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Width(colCursor).Render(""),
		lipgloss.NewStyle().Width(colVisual).Render(""),
		lipgloss.NewStyle().Width(colKind).Render(headerStyle.Render("KIND")),
		lipgloss.NewStyle().Width(colName).Render(headerStyle.Render("NAME")),
		lipgloss.NewStyle().Width(colStatus).Render(headerStyle.Render("STATUS")),
		headerStyle.Render("INFO"),
	)
	headerSeparator := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", w-2))

	var rows []string

	for i, item := range h.inventoryItems {
		isSel := (i == h.inventoryCursor)
		cursor := "    "
		if isSel {
			cursor = " >  "
		}

		cCursor := lipgloss.NewStyle().Width(colCursor).Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(cursor)

		icon := styles.GetIcon(item.GVK.Kind)
		visualTxt := icon
		if item.TreeLine != "" {
			visualTxt = item.TreeLine + " " + icon
		}
		cVisual := h.renderFixedCell(visualTxt, colVisual, lipgloss.Color("#FFFFFF"), false)

		cKind := h.renderFixedCell(item.GVK.Kind, colKind, lipgloss.Color("#00FFFF"), isSel)
		cName := h.renderFixedCell(item.Name, colName, lipgloss.Color("#FFFFFF"), isSel)

		statusColor := lipgloss.Color(styles.ColorDefault)
		if item.ColorHint == "ok" {
			statusColor = lipgloss.Color(styles.ColorRunning)
		}
		if item.ColorHint == "warn" {
			statusColor = lipgloss.Color(styles.ColorPending)
		}
		if item.ColorHint == "error" {
			statusColor = lipgloss.Color(styles.ColorError)
		}
		cStatus := h.renderFixedCell(item.Status, colStatus, statusColor, isSel)

		infoTxt := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Render(item.Impact + " " + item.Info)
		if isSel {
			infoTxt = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AAAAAA")).Render(item.Impact + " " + item.Info)
		}

		row := lipgloss.JoinHorizontal(lipgloss.Left, cCursor, cVisual, cKind, cName, cStatus, infoTxt)
		if isSel {
			row = lipgloss.NewStyle().Background(lipgloss.Color("#222222")).Width(w - 2).Render(row)
		}
		rows = append(rows, row)
	}

	h.resViewport.Width = w - 2
	h.resViewport.Height = height - 4
	h.resViewport.SetContent(strings.Join(rows, "\n"))

	title := " MANAGED RESOURCES (Pointer: <y> YAML, <d> Describe)"
	return border.Render(lipgloss.JoinVertical(lipgloss.Left, title, headerRow, headerSeparator, h.resViewport.View()))
}

func (h *HelmDashboard) renderEventsBox(w, height int) string {
	
	if height < 3 { return "" }
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Width(w).Height(height)
	var rows []string
	if len(h.aggregatedEvents) == 0 {
		rows = append(rows, "No recent events.")
	} else {
		for _, e := range h.aggregatedEvents {
			timeStr := utils.FormatDuration(time.Since(e.LastTime))
			prefix := "i"
			color := lipgloss.Color("#00FF00")
			if e.Type == "Warning" {
				prefix = "!"; color = lipgloss.Color("#FF0000")
			}
			line := fmt.Sprintf("%s [%s] %s: %s (%dx)", prefix, timeStr, e.Reason, e.Object, e.Count)
			rows = append(rows, lipgloss.NewStyle().Foreground(color).Render(line))
		}
	}
	h.evtViewport.Width = w - 2
	h.evtViewport.Height = height - 2
	h.evtViewport.SetContent(strings.Join(rows, "\n"))
	title := " INTELLIGENT EVENT STREAM   Press 9 (Event View) outside of this Dashboard to get more insights"
	return border.Render(lipgloss.JoinVertical(lipgloss.Left, title, h.evtViewport.View()))
}

func (h *HelmDashboard) renderFixedCell(text string, width int, color lipgloss.Color, bold bool) string {
	style := lipgloss.NewStyle().Foreground(color)
	if bold {
		style = style.Bold(true)
	}
	safeW := width - 1
	if lipgloss.Width(text) > safeW {
		runes := []rune(text)
		if len(runes) > safeW {
			text = string(runes[:safeW-1]) + "…"
		}
	}
	return style.Width(width).MaxWidth(width).MaxHeight(1).Render(text)
}

func (h *HelmDashboard) renderValues() string {
	var b strings.Builder
	b.WriteString(styles.ResourceTitleStyle.Render(" 🔧 INTELLIGENT VALUES INSPECTOR ") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Bold(true).Render(" ■ User Override  ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")).Render("■ Default Value") + "\n\n")

	if h.inputMode || h.valuesFilter != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(fmt.Sprintf("Filter: %s█", h.valuesFilter)) + "\n\n")
	}

	computedYaml, _ := yaml.Marshal(h.valuesAll)
	lines := strings.Split(string(computedYaml), "\n")

	type stackItem struct {
		key    string
		indent int
	}
	var pathStack []stackItem

	for _, l := range lines {
		if h.valuesFilter != "" && !strings.Contains(strings.ToLower(l), strings.ToLower(h.valuesFilter)) {
			continue
		}

		cleanLine := strings.TrimSpace(l)
		if cleanLine == "" || strings.HasPrefix(cleanLine, "#") {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(l) + "\n")
			continue
		}

		indent := len(l) - len(strings.TrimLeft(l, " "))
		
		parts := strings.SplitN(cleanLine, ":", 2)
		key := strings.TrimSpace(parts[0])
		
		for len(pathStack) > 0 {
			last := pathStack[len(pathStack)-1]
			if last.indent >= indent {
				pathStack = pathStack[:len(pathStack)-1]
			} else {
				break
			}
		}
		pathStack = append(pathStack, stackItem{key: key, indent: indent})

		fullPath := ""
		for i, item := range pathStack {
			if i > 0 { fullPath += "." }
			fullPath += item.key
		}

		isOverride := false
		if h.userKeys[fullPath] {
			isOverride = true
		} else {
			for p := range h.userKeys {
				if strings.HasPrefix(fullPath, p) {
					isOverride = true
					break
				}
			}
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
		if isOverride {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Bold(true)
		}

		val := ""
		if len(parts) > 1 {
			val = parts[1]
			cleanKey := strings.ToLower(key)
			if strings.Contains(cleanKey, "password") || strings.Contains(cleanKey, "secret") {
				val = " [*** HIDDEN ***]"
			}
		}

		renderedLine := style.Render(l[:indent] + key + ":") + style.Render(val)
		
		if h.valuesFilter != "" {
			idx := strings.Index(strings.ToLower(l), strings.ToLower(h.valuesFilter))
			if idx != -1 {
				highlight := lipgloss.NewStyle().Background(lipgloss.Color("#FFFFFF")).Foreground(lipgloss.Color("#000000")).Render(l[idx : idx+len(h.valuesFilter)])
				renderedLine = style.Render(l[:idx]) + highlight + style.Render(l[idx+len(h.valuesFilter):])
			}
		}

		b.WriteString(renderedLine + "\n")
	}
	return b.String()
}

func flattenMap(prefix string, m map[string]interface{}, res map[string]bool) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		res[key] = true
		
		if nested, ok := v.(map[string]interface{}); ok {
			flattenMap(key, nested, res)
		}
	}
}

func (h *HelmDashboard) renderHistory() string {
	var b strings.Builder
	b.WriteString(styles.ResourceTitleStyle.Render(" 📜 RELEASE TIMELINE (Press <Enter> to Diff vs Current) ") + "\n\n")

	wRev := 8
	wAge := 15
	wAction := 15
	wChart := 25
	wApp := 15
	wStatus := 15

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Width(4).Render(""),
		headerStyle.Width(wRev).Render("REV"),
		headerStyle.Width(wAge).Render("AGE"),
		headerStyle.Width(wAction).Render("ACTION"),
		headerStyle.Width(wChart).Render("CHART"),
		headerStyle.Width(wApp).Render("APP VER"),
		headerStyle.Width(wStatus).Render("STATUS"),
		headerStyle.Render("DESCRIPTION"),
	)

	b.WriteString(header + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", h.width-4)) + "\n")

	for i, r := range h.history {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
		
		connector := "│"
		bullet := "●"
		
		if r.Version == h.release.Version {
			style = style.Copy().Foreground(lipgloss.Color(styles.ColorAccent)).Bold(true)
		}

		cursor := "    "
		rowStyle := lipgloss.NewStyle() 
		
		if i == h.historyCursor {
			cursor = " >  "
			rowStyle = rowStyle.Background(lipgloss.Color(styles.ColorSelected))
			style = style.Copy().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
		}

		statusColor := lipgloss.Color(styles.ColorDefault)
		if r.Info.Status == release.StatusDeployed {
			statusColor = lipgloss.Color(styles.ColorRunning)
		} else if r.Info.Status == release.StatusFailed {
			statusColor = lipgloss.Color(styles.ColorError)
		} else if r.Info.Status == release.StatusSuperseded {
			statusColor = lipgloss.Color("#666666")
		}

		age := utils.ComputeAge(&metav1.Time{Time: r.Info.LastDeployed.Time})

		action := "Upgrade"
		if r.Version == 1 {
			action = "Install"
		}
		if strings.Contains(strings.ToLower(r.Info.Description), "rollback") {
			action = "Rollback"
		}

		if i == len(h.history)-1 {
			connector = " "
		}

		lineContent := lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(4).Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(cursor),
			style.Width(wRev).Render(fmt.Sprintf("%s %s %d", connector, bullet, r.Version)),
			style.Width(wAge).Render(age),
			style.Width(wAction).Render(action),
			style.Width(wChart).Render(r.Chart.Metadata.Version),
			style.Width(wApp).Render(r.Chart.Metadata.AppVersion),
			style.Width(wStatus).Foreground(statusColor).Render(r.Info.Status.String()),
			style.Render(r.Info.Description),
		)

		b.WriteString(rowStyle.Width(h.width - 2).Render(lineContent) + "\n")
	}
	return b.String()
}

func (h *HelmDashboard) renderDiff() string {
	var b strings.Builder
	
	baseVer := "?"
	targetVer := "?"
	if h.diffBase != nil { baseVer = fmt.Sprintf("v%d", h.diffBase.Version) }
	if h.diffTarget != nil { targetVer = fmt.Sprintf("v%d", h.diffTarget.Version) }

	title := fmt.Sprintf(" 🔍 DIFF: %s vs %s ", baseVer, targetVer)
	b.WriteString(styles.ResourceTitleStyle.Render(title) + "\n\n")
	
	if h.diffBase == nil || h.diffTarget == nil {
		b.WriteString("Insufficient data to compare.\n")
		return b.String()
	}
	
	diff := utils.ComputeDiff(
		fmt.Sprintf("Rev %d (%s)", h.diffBase.Version, h.diffBase.Info.Status), h.diffBase.Manifest,
		fmt.Sprintf("Rev %d (%s)", h.diffTarget.Version, h.diffTarget.Info.Status), h.diffTarget.Manifest,
		h.width,
	)
	b.WriteString(diff)
	return b.String()
}

func (h *HelmDashboard) renderDrift() string {
	if h.driftFullscreen {
		header := styles.ResourceTitleStyle.Width(h.width).Background(lipgloss.Color("#FF5555")).Render(" 🔎 DRIFT DETAIL [FULLSCREEN] - Press <f> to Split View ")
		h.driftViewport.Width = h.width
		h.driftViewport.Height = h.height - 4
		return lipgloss.JoinVertical(lipgloss.Left, header, h.driftViewport.View())
	}

	headerText := " ⚡ DRIFT INTELLIGENCE (Manifest vs Live) "
	if h.driftDiffContent != "" {
		headerText += " [Press <f> for Fullscreen]"
	}
	header := styles.ResourceTitleStyle.Width(h.width).Render(headerText)

	if len(h.driftedResources) == 0 {
		msg := lipgloss.NewStyle().Padding(2).Foreground(lipgloss.Color("#00FF00")).Render("✅ Sync Status: 100% Clean. No drift detected.")
		return lipgloss.JoinVertical(lipgloss.Left, header, msg)
	}

	leftWidth := int(float64(h.width) * 0.30)
	rightWidth := h.width - leftWidth - 2
	height := h.height - 4

	var listBuilder strings.Builder
	listBuilder.WriteString(lipgloss.NewStyle().Bold(true).Border(lipgloss.NormalBorder(), false, false, true, false).Width(leftWidth).Render("DRIFTED ITEMS") + "\n")

	for i, r := range h.driftedResources {
		cursor := " "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
		if i == h.driftCursor {
			cursor = "▶"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#005F87")).Bold(true)
		}
		
		kindIcon := styles.GetIcon(r.GVK.Kind)
		line := fmt.Sprintf("%s %s %s/%s", cursor, kindIcon, r.GVK.Kind, r.Name)
		listBuilder.WriteString(style.Width(leftWidth).Render(line) + "\n")
		
		if i == h.driftCursor {
			for _, issue := range r.DriftIssues {
				listBuilder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("  └ "+issue) + "\n")
			}
		}
	}

	leftPanel := lipgloss.NewStyle().
		Width(leftWidth).
		Height(height).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		Render(listBuilder.String())

	var rightPanel string
	if h.driftLoading {
		rightPanel = lipgloss.NewStyle().Align(lipgloss.Center).Width(rightWidth).Render("\n\n⏳ Generating Deep Diff...")
	} else if h.driftDiffContent != "" {
		h.driftViewport.Width = rightWidth
		h.driftViewport.Height = height
		rightPanel = h.driftViewport.View()
	} else {
		rightPanel = lipgloss.NewStyle().
			Align(lipgloss.Center).
			Width(rightWidth).
			Foreground(lipgloss.Color("#666666")).
			Render("\n\n\nSelect a resource on the left and press <Enter> to view exact YAML drift.\n(Auto-Fullscreen)")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel))
}

func (h *HelmDashboard) refreshDriftDiff() {
	if h.driftDesiredYAML == "" || h.driftLiveYAML == "" {
		return
	}
	
	targetWidth := h.width
	if !h.driftFullscreen {
		leftWidth := int(float64(h.width) * 0.30)
		targetWidth = h.width - leftWidth - 2
	}

	h.driftDiffContent = utils.ComputeDiff(
		"Manifest (Desired)", h.driftDesiredYAML,
		"Live Cluster State", h.driftLiveYAML,
		targetWidth,
	)
	h.driftViewport.SetContent(h.driftDiffContent)
}

func (h *HelmDashboard) calculateTimings() {
	if h.release == nil { return }
	h.age = utils.ComputeAge(&metav1.Time{Time: h.release.Info.FirstDeployed.Time})
}

func (h *HelmDashboard) computeHealthAndDrift() {
	if h.analysis == nil { return }
	h.riskScore = h.analysis.RiskScore
	h.blastRadius = h.analysis.BlastRadius
	h.securityCount = h.analysis.SecurityCount
	h.driftStatus = h.analysis.DriftStatus
	h.inventoryItems = h.analysis.ManagedResources
	h.driftedResources = []ManagedResource{}
	for _, r := range h.inventoryItems {
		if r.HasDrift {
			h.driftedResources = append(h.driftedResources, r)
		}
	}
}

func (h *HelmDashboard) calculateDriftDiff(r ManagedResource) tea.Cmd {
	return func() tea.Msg {
		liveObj, err := h.fetchCleanLive(r)
		if err != nil { return driftDiffLoadedMsg{err: err} }
		objs, _ := h.client.ParseManifestToObjects(h.release.Manifest)
		var desiredObj *unstructured.Unstructured
		for _, o := range objs {
			if o.GetKind() == r.GVK.Kind && o.GetName() == r.Name {
				desiredObj = o; break
			}
		}
		if desiredObj == nil { return driftDiffLoadedMsg{err: fmt.Errorf("desired object not found in manifest")} }
		desiredYaml, _ := yaml.Marshal(desiredObj.Object)
		liveYaml, _ := yaml.Marshal(liveObj.Object)
		return driftDiffLoadedMsg{content: "", desired: string(desiredYaml), live: string(liveYaml)}
	}
}

func (h *HelmDashboard) fetchCleanLive(r ManagedResource) (*unstructured.Unstructured, error) {
	mapping, err := h.client.GetMapper().RESTMapping(r.GVK.GroupKind(), r.GVK.Version)
	if err != nil { return nil, err }
	dyn := h.client.GetDynamicClient()
	var obj *unstructured.Unstructured
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		obj, err = dyn.Resource(mapping.Resource).Namespace(r.Namespace).Get(context.TODO(), r.Name, metav1.GetOptions{})
	} else {
		obj, err = dyn.Resource(mapping.Resource).Get(context.TODO(), r.Name, metav1.GetOptions{})
	}
	if err != nil { return nil, err }
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "status")
	return obj, nil
}

func (h *HelmDashboard) processEvents(allEvents []corev1.Event) {
	if h.analysis == nil { return }
	aggMap := make(map[string]*AggregatedEvent)
	for _, e := range h.analysis.Events {
		key := e.InvolvedObject.Name + e.Reason
		if val, exists := aggMap[key]; exists {
			val.Count++
			if e.LastTimestamp.Time.After(val.LastTime) {
				val.LastTime = e.LastTimestamp.Time; val.Message = e.Message
			}
		} else {
			aggMap[key] = &AggregatedEvent{
				Reason: e.Reason, Message: e.Message, Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, Count: 1, Type: e.Type, LastTime: e.LastTimestamp.Time,
			}
		}
	}
	h.aggregatedEvents = make([]AggregatedEvent, 0, len(aggMap))
	for _, v := range aggMap { h.aggregatedEvents = append(h.aggregatedEvents, *v) }
	sort.Slice(h.aggregatedEvents, func(i, j int) bool { return h.aggregatedEvents[i].LastTime.After(h.aggregatedEvents[j].LastTime) })
}

func (h *HelmDashboard) synthesizeOverallHealth() {
	h.healthGrade = "HEALTHY"; h.verdictIcon = "✓"; h.verdictColor = lipgloss.Color(styles.ColorRunning); h.statusSentence = "All Systems Nominal"
	if h.analysis != nil {
		if h.analysis.HealthStatus == "Degraded" {
			h.healthGrade = "DEGRADED"; h.verdictIcon = "⚠"; h.verdictColor = lipgloss.Color(styles.ColorPending); h.statusSentence = "Workload Issues Detected"
		}
		if h.analysis.HealthStatus == "Failed" {
			h.healthGrade = "FAILED"; h.verdictIcon = "✖"; h.verdictColor = lipgloss.Color(styles.ColorError); h.statusSentence = "Critical Failure"
		}
	}
}

func (h *HelmDashboard) copyCLICommand() tea.Cmd {
	cmd := fmt.Sprintf("helm upgrade --install %s %s/%s --version %s -n %s",
		h.releaseName, h.release.Chart.Metadata.Name, h.release.Chart.Metadata.Version, h.release.Chart.Metadata.Version, h.namespace)
	return func() tea.Msg { utils.CopyToClipboard(cmd); return nil }
}

func clamp(v, min, max int) int {
	if v < min { return min }
	if v > max { return max }
	return v
}