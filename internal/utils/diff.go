package utils

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)


func ComputeDiff(nameA, contentA, nameB, contentB string, width int) string {
	linesA := splitLines(contentA)
	linesB := splitLines(contentB)

	var sb strings.Builder

	
	lineNumWidth := 4
	dividerWidth := 3 
	margins := 2

	
	availableWidth := width - (lineNumWidth * 2) - dividerWidth - margins
	if availableWidth < 10 {
		availableWidth = 40 
	}
	halfWidth := availableWidth / 2

	if halfWidth < 20 {
		halfWidth = 20
	}

	
	styleHeader := lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1).
		Width(width).Align(lipgloss.Center)

	styleBase := lipgloss.NewStyle().Width(halfWidth).MaxWidth(halfWidth)

	
	styleRemoved := styleBase.Copy().Background(lipgloss.Color("#3e2e2e")).Foreground(lipgloss.Color("#ff5555"))
	styleAdded := styleBase.Copy().Background(lipgloss.Color("#2e3e2e")).Foreground(lipgloss.Color("#55ff55"))
	styleNormal := styleBase.Copy().Foreground(lipgloss.Color("#999999"))

	styleLineNum := lipgloss.NewStyle().Width(lineNumWidth).Foreground(lipgloss.Color("#444444")).Align(lipgloss.Right).Background(lipgloss.Color("#1a1a1a"))

	styleEmptyNum := lipgloss.NewStyle().Width(lineNumWidth).Background(lipgloss.Color("#101010"))
	styleEmptyText := lipgloss.NewStyle().Width(halfWidth).Background(lipgloss.Color("#101010"))

	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render(" │ ")

	sb.WriteString(styleHeader.Render(fmt.Sprintf("%s ◄──► %s", nameA, nameB)) + "\n")

	matrix := lcsMatrix(linesA, linesB)
	diffs := backtrackLCS(matrix, linesA, linesB)

	lineNumA := 1
	lineNumB := 1

	for _, d := range diffs {
		var leftBlock, rightBlock string

		numA := fmt.Sprintf("%3d ", lineNumA)
		numB := fmt.Sprintf("%3d ", lineNumB)

		
		pad := func(s string, w int) string {
			visW := lipgloss.Width(s)
			if visW > w {
				return s[:w]
			}
			return s + strings.Repeat(" ", w-visW)
		}

		cleanLine := strings.ReplaceAll(d.Line, "\t", "  ")

		switch d.Type {
		case "same":
			txt := pad(cleanLine, halfWidth)
			leftBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleLineNum.Render(numA), styleNormal.Render(txt))
			rightBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleLineNum.Render(numB), styleNormal.Render(txt))
			lineNumA++
			lineNumB++

		case "remove":
			txt := pad("- "+cleanLine, halfWidth)
			leftBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleLineNum.Render(numA), styleRemoved.Render(txt))
			rightBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleEmptyNum.Render(" "), styleEmptyText.Render(" "))
			lineNumA++

		case "add":
			txt := pad("+ "+cleanLine, halfWidth)
			leftBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleEmptyNum.Render(" "), styleEmptyText.Render(" "))
			rightBlock = lipgloss.JoinHorizontal(lipgloss.Left, styleLineNum.Render(numB), styleAdded.Render(txt))
			lineNumB++
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, divider, rightBlock)
		sb.WriteString(row + "\n")
	}

	return sb.String()
}


func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

type diffLine struct {
	Type string
	Line string
}

func lcsMatrix(x, y []string) [][]int {
	m := len(x)
	n := len(y)
	c := make([][]int, m+1)
	for i := range c {
		c[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if strings.TrimSpace(x[i-1]) == strings.TrimSpace(y[j-1]) {
				c[i][j] = c[i-1][j-1] + 1
			} else {
				
				if c[i-1][j] > c[i][j-1] {
					c[i][j] = c[i-1][j] 
				} else {
					c[i][j] = c[i][j-1]
				}
			}
		}
	}
	return c
}


func backtrackLCS(c [][]int, x, y []string) []diffLine {
	i := len(x)
	j := len(y)
	
	
	result := make([]diffLine, 0, i+j)

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && strings.TrimSpace(x[i-1]) == strings.TrimSpace(y[j-1]) {
			result = append(result, diffLine{Type: "same", Line: x[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || c[i][j-1] >= c[i-1][j]) {
			result = append(result, diffLine{Type: "add", Line: y[j-1]})
			j--
		} else if i > 0 && (j == 0 || c[i][j-1] < c[i-1][j]) {
			result = append(result, diffLine{Type: "remove", Line: x[i-1]})
			i--
		}
	}

	
	for k, l := 0, len(result)-1; k < l; k, l = k+1, l-1 {
		result[k], result[l] = result[l], result[k]
	}

	return result
}