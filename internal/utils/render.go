package utils

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)


func RenderUsageBar(current int64, limit int64, formatStr string) string {
	
	if limit <= 0 {
		return fmt.Sprintf(formatStr, current) 
	}

	
	percent := float64(current) / float64(limit)
	if percent > 1.0 {
		percent = 1.0
	}
	
	
	width := 8 
	filled := int(math.Round(percent * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled

	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	
	var color lipgloss.Color
	if percent >= 0.9 {
		color = lipgloss.Color("#FF0000") 
	} else if percent >= 0.75 {
		color = lipgloss.Color("#FFA500") 
	} else {
		color = lipgloss.Color("#00FF00") 
	}


	valStr := fmt.Sprintf(formatStr, current)
	pctStr := fmt.Sprintf("%.0f%%", percent*100)
	

	barStyled := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("[%s] %s", bar, pctStr))
	
	return fmt.Sprintf("%s %s", valStr, barStyled)
}