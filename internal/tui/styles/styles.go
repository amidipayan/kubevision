package styles

import (
	"github.com/charmbracelet/lipgloss"
)


const (
	ColorBackground = "#1A1A1A"
	ColorPrimary    = "#FFFFFF"
	ColorAccent     = "#FFD700" 
	ColorSelected   = "#5A5A5A" 
	ColorHeaderBg   = "#333333" 
	
	
	ColorRunning  = "#00FF00" 
	ColorPending  = "#FFA500" 
	ColorError    = "#FF0000" 
	ColorDefault  = "#808080" 
	ColorTitleBg  = "#0066AA" 
)


var HeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorPrimary)).
	Background(lipgloss.Color(ColorHeaderBg)).
	Padding(0, 1).
	MarginBottom(1)


var FooterStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorAccent)).
	Background(lipgloss.Color(ColorHeaderBg)).
	Padding(0, 1)


var ShortcutKeyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorAccent)).
	Bold(true)


var ResourceTitleStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorPrimary)).
	Background(lipgloss.Color(ColorTitleBg)).
	Padding(0, 1)


var TableHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorAccent)).
	Bold(true).
	Padding(0, 1)


var DefaultRowStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorPrimary)).
	Padding(0, 1)


var SelectedRowStyle = DefaultRowStyle.Copy().
	Background(lipgloss.Color(ColorSelected))


var StatusDefaultStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorDefault))


var StatusRunningStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorRunning)).
	Bold(true)


var StatusPendingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorPending)).
	Bold(true)


var StatusErrorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorError)).
	Bold(true)


var YAMLHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(ColorPrimary)).
	Background(lipgloss.Color("#553388")).
	Padding(0, 1)


var TabBaseStyle = lipgloss.NewStyle().
	Padding(0, 2).
	Border(lipgloss.NormalBorder(), false, true, false, false). 
	BorderForeground(lipgloss.Color("#555555"))


var TabInactiveStyle = TabBaseStyle.Copy().
	Foreground(lipgloss.Color("#FFFFFF")). 
	Background(lipgloss.Color("#444444"))  


var TabActiveStyle = TabBaseStyle.Copy().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(lipgloss.Color("#005F87")). 
	Bold(true)


var TabGapStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#222222"))



var ResourceIcons = map[string]string{
	"Pod":                            "📦",
	"Service":                        "🔌",
	"Deployment":                     "🚀",
	"ReplicaSet":                     "👯",
	"StatefulSet":                    "🏢",
	"DaemonSet":                      "👻",
	"ConfigMap":                      "📝",
	"Secret":                         "🔒",
	"Ingress":                        "🌐",
	"IngressClass":                   "🚪",
	"Role":                           "🔑", 
	"ClusterRole":                    "🔑", 
	"RoleBinding":                    "🔗", 
	"ClusterRoleBinding":             "🔗", 
	"ServiceAccount":                 "👤",
	"PersistentVolumeClaim":          "💾",
	"PersistentVolume":               "💽",
	"StorageClass":                   "🗄️",
	"NetworkPolicy":                  "🚧",
	"ValidatingWebhookConfiguration": "👮",
	"MutatingWebhookConfiguration":   "👽",
	"Job":                            "⏳",
	"CronJob":                        "⏰",
	"Event":                          "🔔",
	"Node":                           "💻",
}


func GetIcon(kind string) string {
	if icon, ok := ResourceIcons[kind]; ok {
		return icon
	}
	return "🔹" 
}