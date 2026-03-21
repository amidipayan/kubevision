package tree

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)


type LayoutStrategy int

const (
	LayoutVerticalTree LayoutStrategy = iota 
	LayoutInvertedTree                       
	LayoutLayered                            
)


const (
	BoxWidth  = 24
	BoxHeight = 4
	GapX      = 6 
	GapY      = 3 
)


var (
	
	styleRoot = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("#00FFFF")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Align(lipgloss.Center).
		Width(BoxWidth - 2)

	
	styleDirect = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFFFFF")).
		Foreground(lipgloss.Color("#EEEEEE")).
		Align(lipgloss.Center).
		Width(BoxWidth - 2)

	
	styleDim = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Foreground(lipgloss.Color("#666666")).
		Align(lipgloss.Center).
		Width(BoxWidth - 2)

	
	styleBlastCritical = styleDirect.Copy().
				BorderForeground(lipgloss.Color("#FF0000")).
				Foreground(lipgloss.Color("#FF0000")).
				Bold(true)

	
	styleBlastHigh = styleDirect.Copy().
			BorderForeground(lipgloss.Color("#FFA500")).
			Foreground(lipgloss.Color("#FFA500"))
)


func RenderGraph(g *TopologyGraph, screenWidth int, strategy LayoutStrategy) string {
	if g == nil || len(g.Nodes) == 0 {
		return lipgloss.NewStyle().
			Align(lipgloss.Center).
			Width(screenWidth).
			Padding(2).
			Foreground(lipgloss.Color("#555555")).
			Render("No Topology Data Available")
	}

	
	levels := computePositions(g, strategy)

	
	placedNodes := packNodes(levels, screenWidth)

	
	return drawCanvas(g, placedNodes, screenWidth, strategy)
}



func computePositions(g *TopologyGraph, strategy LayoutStrategy) map[int][]*TopologyNode {
	if strategy == LayoutLayered {
		return computeLevelsLayered(g)
	}
	return computeLevelsBFS(g)
}


func computeLevelsLayered(g *TopologyGraph) map[int][]*TopologyNode {
	levels := make(map[int][]*TopologyNode)

	
	const (
		ColExternal = 0 
		ColNetwork  = 1 
		ColService  = 2 
		ColCompute  = 3 
		ColStorage  = 4 
		ColConfig   = 5 
	)

	for _, n := range g.Nodes {
		col := ColCompute 
		switch n.Kind {
		case KindExternal:
			col = ColExternal
		case KindNetwork:
			col = ColNetwork
		case KindService:
			col = ColService
		case KindWorkload:
			col = ColCompute
		case KindInfra:
			col = ColStorage
		case KindConfig, KindGroup:
			col = ColConfig
		}

		
		n.LevelIdx = col
		levels[col] = append(levels[col], n)
	}

	return levels
}


func computeLevelsBFS(g *TopologyGraph) map[int][]*TopologyNode {
	levels := make(map[int][]*TopologyNode)
	visited := make(map[string]bool)

	root, exists := g.Nodes[g.RootID]
	if !exists {
		return levels
	}

	
	type queueItem struct {
		node  *TopologyNode
		level int
	}
	queue := []queueItem{{root, 0}}
	visited[root.ID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		curr.node.LevelIdx = curr.level
		levels[curr.level] = append(levels[curr.level], curr.node)

		
		for _, edge := range g.Edges {
			var nextNode *TopologyNode
			var nextLevel int

			
			if edge.FromID == curr.node.ID {
				nextNode = g.Nodes[edge.ToID]
				nextLevel = curr.level + 1
			} else if edge.ToID == curr.node.ID {
				
				nextNode = g.Nodes[edge.FromID]
				nextLevel = curr.level - 1
			}

			if nextNode != nil && !visited[nextNode.ID] {
				visited[nextNode.ID] = true
				queue = append(queue, queueItem{nextNode, nextLevel})
			}
		}
	}
	return levels
}



func packNodes(levels map[int][]*TopologyNode, screenWidth int) []*TopologyNode {
	var placed []*TopologyNode

	minLevel, maxLevel := 9999, -9999
	for l := range levels {
		if l < minLevel { minLevel = l }
		if l > maxLevel { maxLevel = l }
	}

	
	currentY := 2

	for l := minLevel; l <= maxLevel; l++ {
		nodesInRow := levels[l]
		if len(nodesInRow) == 0 {
			continue
		}

		
		totalRowWidth := len(nodesInRow)*BoxWidth + (len(nodesInRow)-1)*GapX
		startX := (screenWidth - totalRowWidth) / 2

		
		if startX < 1 {
			startX = 1
		}

		for i, n := range nodesInRow {
			n.Width = BoxWidth
			n.Height = BoxHeight
			n.X = startX + i*(BoxWidth+GapX)
			n.Y = currentY

			placed = append(placed, n)
		}

		// Move down for next level
		currentY += BoxHeight + GapY
	}
	return placed
}


type Canvas struct {
	Grid   [][]string
	Width  int
	Height int
}

func newCanvas(w, h int) *Canvas {
	grid := make([][]string, h)
	for i := range grid {
		grid[i] = make([]string, w)
		for j := range grid[i] {
			grid[i][j] = " "
		}
	}
	return &Canvas{Grid: grid, Width: w, Height: h}
}

func (c *Canvas) Set(x, y int, char string) {
	if x >= 0 && x < c.Width && y >= 0 && y < c.Height {
		c.Grid[y][x] = char
	}
}


func drawCanvas(g *TopologyGraph, nodes []*TopologyNode, width int, strategy LayoutStrategy) string {
	
	maxHeight := 0
	for _, n := range nodes {
		if bottom := n.Y + n.Height; bottom > maxHeight {
			maxHeight = bottom
		}
	}
	height := maxHeight + 4 
	if height < 20 {
		height = 20
	}

	canvas := newCanvas(width, height)

	
	styleEdge := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")) 

	
	for _, edge := range g.Edges {
		from, ok1 := g.Nodes[edge.FromID]
		to, ok2 := g.Nodes[edge.ToID]

		
		if !ok1 || !ok2 || from.Width == 0 || to.Width == 0 {
			continue
		}

		
		x1 := from.X + (from.Width / 2)
		y1 := from.Y + from.Height
		x2 := to.X + (to.Width / 2)
		y2 := to.Y

		
		midY := y1 + (y2-y1)/2
		for y := y1; y <= midY; y++ {
			canvas.Set(x1, y, styleEdge.Render("│"))
		}

		
		startX, endX := x1, x2
		if x2 < x1 {
			startX, endX = x2, x1
		}

		for x := startX; x <= endX; x++ {
			char := "─"
			
			if x == x1 && x != x2 {
				if midY > y1 {
					char = "└"
				}
				if x1 > x2 {
					char = "┘"
				}
			}
			if x == x2 && x != x1 {
				if x2 > x1 {
					char = "┐"
				}
				if x2 < x1 {
					char = "┌"
				}
			}
			canvas.Set(x, midY, styleEdge.Render(char))
		}

		
		for y := midY; y < y2; y++ {
			if y == midY {
				if x1 == x2 {
					canvas.Set(x2, y, styleEdge.Render("│"))
				}
				continue
			}
			canvas.Set(x2, y, styleEdge.Render("│"))
		}

		canvas.Set(x2, y2-1, styleEdge.Render("▼"))
	}

	
	for _, n := range nodes {
		drawNodeBox(canvas, n, strategy)
	}

	
	legend := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(
		"Legend: [L1] Target  [L2] Critical  [L3] Config  │  Layout: [Tab]  Quit: [Esc]")

	var sb strings.Builder
	sb.WriteString(legend + "\n\n") 
	for y := 0; y < canvas.Height; y++ {
		for x := 0; x < canvas.Width; x++ {
			sb.WriteString(canvas.Grid[y][x])
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func drawNodeBox(c *Canvas, n *TopologyNode, strategy LayoutStrategy) {
	
	style := styleDim

	
	if n.Level == LevelRoot {
		style = styleRoot
	} else if n.Level == LevelCritical {
		style = styleDirect
	}

	
	if strategy == LayoutInvertedTree && n.Level != LevelRoot {
		
		if n.Kind == KindInfra || n.Criticality == TierCritical {
			style = styleBlastCritical
		} else {
			style = styleBlastHigh
		}
	}

	
	kindShort := n.Kind.String()
	if n.IsGroup {
		kindShort = fmt.Sprintf("%d items", n.GroupCount)
	}

	content := fmt.Sprintf("%s %s\n%s", n.Icon, truncate(n.Name, 16), kindShort)

	
	renderedBox := style.Render(content)
	lines := strings.Split(renderedBox, "\n")

	
	for i, line := range lines {
		if n.Y+i < c.Height {
			c.Set(n.X, n.Y+i, line)
			
			for j := 1; j < BoxWidth; j++ {
				c.Set(n.X+j, n.Y+i, "")
			}
		}
	}
}



func (k NodeKind) String() string {
	switch k {
	case KindWorkload:
		return "Workload"
	case KindService:
		return "Service"
	case KindNetwork:
		return "Network"
	case KindConfig:
		return "Config"
	case KindInfra:
		return "Infra"
	case KindExternal:
		return "External"
	case KindGroup:
		return "Group"
	default:
		return "Unknown"
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}