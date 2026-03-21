package tree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)


type Model struct {
	
	Width  int
	Height int

	
	Root     *Node   
	Cursor   int     
	flatList []*Node 

	
	Graph        *TopologyGraph
	ActiveLayout LayoutStrategy
	IsGraphMode  bool
}

func NewModel(width, height int) Model {
	return Model{
		Width:  width,
		Height: height,
	}
}


func (m *Model) SetRoot(root *Node) {
	m.Root = root
	m.IsGraphMode = false
	
	
	if m.Root != nil {
		m.Root.Expanded = true
	}
	m.Cursor = 0
	m.rebuildFlatList()
}


func (m *Model) SetGraph(g *TopologyGraph) {
	m.Graph = g
	m.IsGraphMode = true
	m.ActiveLayout = LayoutVerticalTree 
}


func (m Model) Init() tea.Cmd {
	return nil
}



func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		
		
		case "tab":
			if m.IsGraphMode {
				
				m.ActiveLayout++
				if m.ActiveLayout > LayoutLayered {
					m.ActiveLayout = LayoutVerticalTree
				}
				return m, nil
			}

		
		case "up", "k":
			if !m.IsGraphMode && m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if !m.IsGraphMode && m.Cursor < len(m.flatList)-1 {
				m.Cursor++
			}
		
		
		case "enter", "space", "right", "l":
			if !m.IsGraphMode && len(m.flatList) > 0 {
				node := m.flatList[m.Cursor]
				if len(node.Children) > 0 {
					node.Expanded = !node.Expanded
					m.rebuildFlatList()
				}
			}
		case "left", "h":
			if !m.IsGraphMode && len(m.flatList) > 0 {
				node := m.flatList[m.Cursor]
				if node.Expanded {
					node.Expanded = false
					m.rebuildFlatList()
				} else if node.Parent != nil {
					
					for i, n := range m.flatList {
						if n == node.Parent {
							m.Cursor = i
							break
						}
					}
				}
			}
		}
	}
	return m, nil
}



func (m Model) View() string {
	
	if m.IsGraphMode {
		header := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")).
			Render(fmt.Sprintf("X-RAY MODE: %s (Tab to switch layout)", getLayoutName(m.ActiveLayout)))
		
		graphView := RenderGraph(m.Graph, m.Width, m.ActiveLayout)
		return fmt.Sprintf("%s\n\n%s", header, graphView)
	}

	
	if m.Root == nil {
		return "Loading Topology..."
	}

	var s strings.Builder
	
	
	start := 0
	end := len(m.flatList)
	height := m.Height - 1 

	if m.Cursor >= height {
		start = m.Cursor - height + 1
	}
	if end > start+height {
		end = start + height
	}

	for i := start; i < end; i++ {
		node := m.flatList[i]
		
		
		prefix := strings.Repeat("  ", node.depth)
		connector := "├─"
		if node.isLast {
			connector = "└─"
		}
		if node.depth == 0 {
			connector = ""
		}

		arrow := ""
		if len(node.Children) > 0 {
			if node.Expanded {
				arrow = "▼ "
			} else {
				arrow = "▶ "
			}
		} else {
			arrow = "  "
		}

		
		style := lipgloss.NewStyle()
		if i == m.Cursor {
			style = style.Background(lipgloss.Color("#444444")).Foreground(lipgloss.Color("#FFFFFF"))
		}
		
		kindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		
		
		statusColor := lipgloss.Color("#FFFFFF")
		sl := strings.ToLower(node.Status)
		if strings.Contains(sl, "run") || strings.Contains(sl, "ready") || strings.Contains(sl, "bound") || strings.Contains(sl, "active") {
			statusColor = lipgloss.Color("#00FF00") 
		} else if strings.Contains(sl, "err") || strings.Contains(sl, "fail") || strings.Contains(sl, "crash") || strings.Contains(sl, "backoff") || strings.Contains(sl, "oom") {
			statusColor = lipgloss.Color("#FF0000") 
		} else if strings.Contains(sl, "pending") || strings.Contains(sl, "containercreating") || strings.Contains(sl, "terminating") {
			statusColor = lipgloss.Color("#FFA500") 
		}


		
		lineBase := fmt.Sprintf("%s%s%s%s ", prefix, connector, arrow, node.Icon)
		
		kindStr := ""
		if node.Kind != "" && node.Kind != "Virtual" {
			kindStr = kindStyle.Render(node.Kind) + " "
		}
		
		statusStr := ""
		if node.Status != "" {
			statusStr = lipgloss.NewStyle().Foreground(statusColor).Render(fmt.Sprintf("[%s]", node.Status))
		}

		
		row := fmt.Sprintf("%s%s%s %s", lineBase, kindStr, node.Name, statusStr)
		s.WriteString(style.Render(row) + "\n")
	}

	return s.String()
}


func getLayoutName(l LayoutStrategy) string {
	switch l {
	case LayoutVerticalTree: return "Vertical Tree"
	case LayoutInvertedTree: return "Blast Radius (Inverted)"
	case LayoutLayered: return "Layered Columns (Datadog Style)"
	default: return "Unknown"
	}
}


func (m *Model) rebuildFlatList() {
	m.flatList = make([]*Node, 0)
	if m.Root == nil {
		return
	}
	m.flatten(m.Root, 0, true)
}

func (m *Model) flatten(node *Node, depth int, isLast bool) {
	node.depth = depth
	node.isLast = isLast
	m.flatList = append(m.flatList, node)

	if node.Expanded {
		for i, child := range node.Children {
			isLastChild := i == len(node.Children)-1
			m.flatten(child, depth+1, isLastChild)
		}
	}
}