package yaml

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/tui/styles"

	// K8s Imports
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
)


type YAMLConfig struct {
	K8sClient    *client.KubeClient
	Namespace    string
	Name         string
	ResourceType string
}


type yamlLoadedMsg struct {
	content string
	err     error
}


type ReturnToExplorerMsg struct{}


type YAMLViewer struct {
	config        YAMLConfig
	yamlContent   string 
	syntaxContent string 
	loading       bool
	errorMsg      string
	width         int
	height        int
	scrollOffset  int
	searchTerm    string
	mode          string 
}


func NewYAMLViewer(cfg YAMLConfig) *YAMLViewer {
	return &YAMLViewer{
		config:  cfg,
		loading: true,
		mode:    "view",
	}
}

func toYAML(obj runtime.Object) ([]byte, error) {
	if obj == nil {
		return nil, fmt.Errorf("object is nil")
	}


	if accessor, err := meta.Accessor(obj); err == nil {
		accessor.SetManagedFields(nil)
	}

	printer := printers.YAMLPrinter{}
	var buf bytes.Buffer

	
	if u, ok := obj.(*unstructured.Unstructured); ok {
		if u.GetAPIVersion() == "" || u.GetKind() == "" {
			gvks, _, _ := scheme.Scheme.ObjectKinds(obj)
			if len(gvks) > 0 {
				u.SetGroupVersionKind(gvks[0])
			}
		}
	}

	err := printer.PrintObj(obj, &buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}


func loadYAML(cfg YAMLConfig) tea.Cmd {
	return func() tea.Msg {
	
		content, err := fetchNative(cfg)
		if err == nil {
			return yamlLoadedMsg{content: content}
		}


		content, err = fetchKubectl(cfg)
		if err != nil {
			return yamlLoadedMsg{err: fmt.Errorf("failed to fetch yaml: %v", err)}
		}

		return yamlLoadedMsg{content: content}
	}
}

func fetchNative(cfg YAMLConfig) (string, error) {
	dynClient := cfg.K8sClient.GetDynamicClient()
	mapper := cfg.K8sClient.GetMapper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	
	gvk := schema.GroupVersionKind{Kind: cfg.ResourceType}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		mapping, err = mapper.RESTMapping(schema.GroupKind{Kind: cfg.ResourceType})
		if err != nil {
			return "", err
		}
	}

	
	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if cfg.Namespace == "" {
			return "", fmt.Errorf("namespace required")
		}
		dri = dynClient.Resource(mapping.Resource).Namespace(cfg.Namespace)
	} else {
		dri = dynClient.Resource(mapping.Resource)
	}

	
	obj, err := dri.Get(ctx, cfg.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}


	bytes, err := toYAML(obj)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func fetchKubectl(cfg YAMLConfig) (string, error) {
	args := []string{"get", cfg.ResourceType, cfg.Name, "-o", "yaml"}
	if cfg.Namespace != "" {
		args = append(args, "-n", cfg.Namespace)
	}

	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(out))
	}
	return string(out), nil
}


func (y *YAMLViewer) Init() tea.Cmd {
	return loadYAML(y.config)
}


func preComputeSyntaxHighlights(content string) string {
	var highlightedBuf bytes.Buffer
	err := quick.Highlight(
		&highlightedBuf,
		content,
		"yaml",
		"terminal256",
		"monokai",
	)
	if err != nil {
		return content 
	}
	return highlightedBuf.String()
}


func (y *YAMLViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		y.width = msg.Width
		y.height = msg.Height
		return y, nil

	case yamlLoadedMsg:
		if msg.err != nil {
			y.setError(msg.err.Error())
		} else {
			y.yamlContent = msg.content
			
			y.syntaxContent = preComputeSyntaxHighlights(msg.content)
			y.loading = false
		}
		return y, nil

	case tea.KeyMsg:
		switch y.mode {
		case "search":
			switch msg.String() {
			case "esc":
				y.mode = "view"
				y.searchTerm = ""
			case "enter":
				y.mode = "view"
			
			case "backspace":
				if len(y.searchTerm) > 0 {
					y.searchTerm = y.searchTerm[:len([]rune(y.searchTerm))-1]
					y.scrollToFirstMatch()
				}
			default:
				if msg.Type == tea.KeyRunes {
					y.searchTerm += string(msg.Runes)
					if !y.loading && y.yamlContent != "" {
						y.scrollToFirstMatch()
					}
				}
			}
		default: 
			switch msg.String() {
			case "q", "esc":
				return y, func() tea.Msg { return ReturnToExplorerMsg{} }
			case "up", "k":
				if y.scrollOffset > 0 {
					y.scrollOffset--
				}
			case "down", "j":
				lines := strings.Split(y.yamlContent, "\n")
				visibleLines := y.height - 5
				maxOffset := len(lines) - visibleLines
				if maxOffset < 0 {
					maxOffset = 0
				}
				if y.scrollOffset < maxOffset {
					y.scrollOffset++
				}
			case "pgup":
				y.scrollOffset -= (y.height - 5)
				if y.scrollOffset < 0 {
					y.scrollOffset = 0
				}
			case "pgdown":
				lines := strings.Split(y.yamlContent, "\n")
				visibleLines := y.height - 5
				maxOffset := len(lines) - visibleLines
				y.scrollOffset += visibleLines
				if y.scrollOffset > maxOffset {
					y.scrollOffset = maxOffset
				}
				if y.scrollOffset < 0 {
					y.scrollOffset = 0
				}
			case "/":
				y.mode = "search"
				y.searchTerm = ""
			}
		}
	}

	return y, nil
}

func (y *YAMLViewer) scrollToFirstMatch() {
	if y.searchTerm == "" || y.loading {
		return
	}
	searchTermLower := strings.ToLower(y.searchTerm)
	lines := strings.Split(y.yamlContent, "\n")

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), searchTermLower) {
			y.scrollOffset = i
			
			if y.scrollOffset > 5 {
				y.scrollOffset -= 2
			}
			if y.scrollOffset < 0 {
				y.scrollOffset = 0
			}
			return
		}
	}
}


func (y *YAMLViewer) View() string {
	var b strings.Builder
	c := y.config

	
	header := fmt.Sprintf(" 📝 YAML Viewer: %s/%s (%s)", c.Namespace, c.Name, c.ResourceType)
	b.WriteString(styles.YAMLHeaderStyle.Width(y.width).Render(header))
	b.WriteString("\n\n")

	
	if y.loading {
		loadingMsg := " ⏳ Loading YAML..."
		b.WriteString(styles.StatusDefaultStyle.Render(loadingMsg))
	} else if y.errorMsg != "" {
		b.WriteString(styles.StatusErrorStyle.Render(" ❌ Error: " + y.errorMsg + "\n"))
	} else {
		b.WriteString(y.renderYAML())
	}

	
	footerText := ""
	if y.mode == "search" {
		footerText = fmt.Sprintf(" Search: %s█ ", y.searchTerm)
	} else {
		footerText = "[q/Esc] Back | [↑↓ PgUp/PgDn] Scroll | [/] Search"
		if y.searchTerm != "" {
			footerText += lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render(fmt.Sprintf(" (Filtering: '%s')", y.searchTerm))
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.FooterStyle.Width(y.width).Render(footerText))

	return b.String()
}

func (y *YAMLViewer) renderYAML() string {
	
	coloredLines := strings.Split(y.syntaxContent, "\n")
	originalLines := strings.Split(y.yamlContent, "\n")

	var result strings.Builder

	visibleLines := y.height - 5
	start := y.scrollOffset
	end := start + visibleLines
	if end > len(coloredLines) {
		end = len(coloredLines)
	}

	searchTermLower := strings.ToLower(y.searchTerm)
	hasSearchTerm := searchTermLower != ""


	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#005F87")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true)

	for i := start; i < end; i++ {
		if i >= len(coloredLines) {
			break
		}

		line := coloredLines[i]

	
		if hasSearchTerm && i < len(originalLines) {
			originalLine := originalLines[i]
			if strings.Contains(strings.ToLower(originalLine), searchTermLower) {

				line = highlightStyle.Render(originalLine)
			}
		}

		result.WriteString(line + "\n")
	}

	return result.String()
}

func (y *YAMLViewer) setError(msg string) {
	y.errorMsg = msg
	y.loading = false
}