package explorer

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)


func (m *ExplorerModel) populateResourceList() {
	var keys []string
	for k := range m.views {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	m.resourceList = keys
	m.filteredResources = keys
	m.resourceIdx = 0
}


func (m *ExplorerModel) filterResourceList() {
	term := strings.ToLower(m.filterInput.Value())
	if term == "" {
		m.filteredResources = m.resourceList
		
		m.resourceIdx = 0
		return
	}

	var matches []string
	for _, r := range m.resourceList {
	
		if strings.Contains(strings.ToLower(r), term) {
			matches = append(matches, r)
		}
	}
	m.filteredResources = matches
	m.resourceIdx = 0
}


func (m *ExplorerModel) updateResourcePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.showResources = false
			m.filterInput.Blur()
			m.statusMsg = "Cancelled resource switch"
			return m, nil

		case tea.KeyUp:
			if m.resourceIdx > 0 {
				m.resourceIdx--
			}
			return m, nil

		case tea.KeyDown:
			if m.resourceIdx < len(m.filteredResources)-1 {
				m.resourceIdx++
			}
			return m, nil

		case tea.KeyEnter:
			if len(m.filteredResources) > 0 {
				selected := m.filteredResources[m.resourceIdx]
				m.showResources = false
				m.filterInput.Blur()

				// Switch View logic
				if view, exists := m.views[selected]; exists {
					m.switchView(selected, view.Title())
					m.statusMsg = fmt.Sprintf("Switched to %s", view.Title())
				}
			}
			return m, nil
		}
	}

	
	oldValue := m.filterInput.Value()
	var cmd tea.Cmd
	
	m.filterInput, cmd = m.filterInput.Update(msg)


	if m.filterInput.Value() != oldValue {
		m.filterResourceList()
	}

	return m, cmd
}


func (m *ExplorerModel) viewResourcePicker() string {
	var b strings.Builder

	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("#553388")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Width(50).
		Align(lipgloss.Center)

	itemStyle := lipgloss.NewStyle().Width(50)
	selectedStyle := itemStyle.Copy().
		Background(lipgloss.Color("#FFA500")).
		Foreground(lipgloss.Color("#000000"))


	b.WriteString(headerStyle.Render(" API DISCOVERY MODE ") + "\n\n")

	
	b.WriteString(lipgloss.NewStyle().Width(50).Background(lipgloss.Color("#222222")).Render(m.filterInput.View()) + "\n")
	b.WriteString(strings.Repeat("─", 50) + "\n")

	
	visibleItems := 15
	start := 0
	if m.resourceIdx > 10 {
		start = m.resourceIdx - 10
	}
	end := start + visibleItems
	if end > len(m.filteredResources) {
		end = len(m.filteredResources)
	}

	if len(m.filteredResources) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  (No resources found)") + "\n")
	} else {
		for i := start; i < end; i++ {
			res := m.filteredResources[i]

			
			icon := "🔹"
			if _, ok := m.views[res].(*GenericView); ok {
				icon = "📄"
			}

			line := fmt.Sprintf("  %s %s", icon, res)

			if i == m.resourceIdx {
				b.WriteString(selectedStyle.Render("> " + res) + "\n")
			} else {
				b.WriteString(itemStyle.Render(line) + "\n")
			}
		}
	}


	b.WriteString(strings.Repeat("─", 50) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf(" %d resources available", len(m.filteredResources))))

	
	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#553388")).
		Padding(1).
		Render(b.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}