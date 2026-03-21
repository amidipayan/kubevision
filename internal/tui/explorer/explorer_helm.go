package explorer

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/amidipayan/kubevision/internal/k8s/helm"
	"github.com/amidipayan/kubevision/internal/tui/styles"
	k8syamlviewer "github.com/amidipayan/kubevision/internal/tui/yaml"
	"github.com/amidipayan/kubevision/internal/utils"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)


type blastRadiusMsg struct {
	report helm.BlastRadius
	err    error
}


type sreAnalysisReadyMsg struct {
	Report helm.SREAnalysis
}


func (m *ExplorerModel) handleHelmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return nil, nil
	}
	sel := m.resources[m.selectedIdx]

	switch msg.String() {
	
	case "enter", "i":
		return m.openDashboard(sel, TabOverview)
	case "v":
		return m.openDashboard(sel, TabValues)
	
	
	case "?":
		return m.openDashboard(sel, TabHistory)
	
	case "C":
		return m.openDashboard(sel, TabDiff)
	case "W":
		return m.openDashboard(sel, TabDrift)
	case "K":
		return m.openDashboard(sel, TabSecurity)

	
	case "e": 
		m.statusMsg = fmt.Sprintf("Preparing Upgrade for %s...", sel.Name)
		m.activeEditResource = sel
		return m.editHelmValues()

	case "B": 
		m.statusMsg = fmt.Sprintf("Calculating rollback impact for %s...", sel.Name)
		return m, m.prepareRollbackDiff(sel)

	case "X": 
		m.statusMsg = fmt.Sprintf("Calculating Blast Radius for %s...", sel.Name)
		return m, m.prepareUninstall(sel)

	
	case "H": 
		m.statusMsg = fmt.Sprintf("🧠 Running SRE Heuristics on %s...", sel.Name)
		return m, m.triggerSREAnalysis()

	case "m": 
		return m.viewHelmManifest()
	case "r": 
		return m.restartHelmWorkloads(sel)
	}
	return nil, nil
}


func (m *ExplorerModel) openDashboard(r Resource, startTab int) (tea.Model, tea.Cmd) {

	dashHeight := m.height - 6
	if dashHeight < 10 {
		dashHeight = 10 
	}

	m.helmDashboard = NewHelmDashboard(m.k8sClient, r.Namespace, r.Name, m.width, dashHeight)
	m.helmDashboard.activeTab = startTab
	m.showHelmDash = true

	
	m.statusMsg = ""

	return m, m.helmDashboard.Init()
}


func (m *ExplorerModel) updateHelmDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.showHelmDash || m.helmDashboard == nil {
		return m, nil
	}

	
	if winMsg, ok := msg.(tea.WindowSizeMsg); ok {
		msg = tea.WindowSizeMsg{
			Width:  winMsg.Width,
			Height: winMsg.Height - 6,
		}
	}

	switch msg := msg.(type) {

	
	case DashResourceSelectMsg:
		r := msg.Resource
		return m.viewGenericDescribe(r.GVK.Kind, r.Name, r.Namespace)

	case tea.KeyMsg:
		if !m.helmDashboard.inputMode {
			switch msg.String() {
			case "esc":
				
				if m.helmDashboard.activeTab == TabDiff ||
					m.helmDashboard.activeTab == TabDrift ||
					m.helmDashboard.activeTab == TabSecurity {
					m.helmDashboard.activeTab = TabOverview
					return m, nil
				}

				m.showHelmDash = false
				m.statusMsg = "Exited Helm Dashboard"
				m.refreshData()
				return m, nil

			
			case "y":
				if m.helmDashboard.activeTab == TabOverview && len(m.helmDashboard.inventoryItems) > 0 {
					sel := m.helmDashboard.inventoryItems[m.helmDashboard.inventoryCursor]
					cfg := k8syamlviewer.YAMLConfig{
						K8sClient:    m.k8sClient,
						Namespace:    sel.Namespace,
						Name:         sel.Name,
						ResourceType: sel.GVK.Kind,
					}
					m.yamlViewer = k8syamlviewer.NewYAMLViewer(cfg)
					m.showYAML = true
					return m, m.yamlViewer.Init()
				}

			
			case "d":
				if m.helmDashboard.activeTab == TabOverview && len(m.helmDashboard.inventoryItems) > 0 {
					sel := m.helmDashboard.inventoryItems[m.helmDashboard.inventoryCursor]
					return m.viewGenericDescribe(sel.GVK.Kind, sel.Name, sel.Namespace)
				}

			
			case "e":
				m.statusMsg = "Opening Editor..."
				res := Resource{
					Name:      m.helmDashboard.releaseName,
					Namespace: m.helmDashboard.namespace,
					Kind:      "HelmRelease",
				}
				m.activeEditResource = res
				return m.editHelmValues()
			}
		}
	}

	var cmd tea.Cmd
	var newModel tea.Model
	newModel, cmd = m.helmDashboard.Update(msg)
	m.helmDashboard = newModel.(*HelmDashboard)
	return m, cmd
}


func (m *ExplorerModel) viewGenericDescribe(kind, name, ns string) (tea.Model, tea.Cmd) {
	cleanKind := strings.TrimSpace(kind)
	if cleanKind == "" {
		m.statusMsg = "Error: Cannot describe resource (Unknown Kind)"
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("Describing %s/%s...", cleanKind, name)

	return m, func() tea.Msg {
		args := []string{"describe", strings.ToLower(cleanKind), name}
		if ns != "" {
			args = append(args, "-n", ns)
		}

		cmd := exec.Command("kubectl", args...)
		out, err := cmd.CombinedOutput()

		content := string(out)
		if err != nil {
			content = fmt.Sprintf("Error running describe: %v\n\nCommand: kubectl %v\n\nOutput:\n%s", err, args, string(out))
		}

		return textWindowMsg{
			title:   fmt.Sprintf("Describe: %s/%s", cleanKind, name),
			content: content,
			status:  "Describe Loaded",
		}
	}
}


func (m *ExplorerModel) viewHelmManifest() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	selected := m.resources[m.selectedIdx]
	m.statusMsg = fmt.Sprintf("Fetching manifest for %s...", selected.Name)

	return m, func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(selected.Namespace)
		if err != nil {
			return textWindowMsg{title: "Error", content: err.Error(), status: "Error"}
		}
		client := action.NewGet(cfg)
		rel, err := client.Run(selected.Name)
		if err != nil {
			return textWindowMsg{title: "Error", content: err.Error(), status: "Error"}
		}
		return textWindowMsg{
			title:   fmt.Sprintf("Helm Manifest: %s (v%d)", selected.Name, rel.Version),
			content: rel.Manifest,
			status:  "Manifest loaded.",
		}
	}
}


func (m *ExplorerModel) editHelmValues() (tea.Model, tea.Cmd) {
	if m.activeEditResource.Name == "" && len(m.resources) > 0 {
		m.activeEditResource = m.resources[m.selectedIdx]
	}
	r := m.activeEditResource
	m.statusMsg = fmt.Sprintf("Fetching values for %s...", r.Name)

	return m, func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(r.Namespace)
		if err != nil {
			return editorFinishedMsg{err: err}
		}

		getValues := action.NewGetValues(cfg)
		
		getValues.AllValues = true

		type result struct {
			vals map[string]interface{}
			err  error
		}
		ch := make(chan result, 1)

		go func() {
			vals, err := getValues.Run(r.Name)
			ch <- result{vals, err}
		}()

		select {
		case res := <-ch:
			if res.err != nil {
				return editorFinishedMsg{err: res.err}
			}

			out, err := yaml.Marshal(res.vals)
			if err != nil {
				return editorFinishedMsg{err: fmt.Errorf("failed to marshal yaml: %w", err)}
			}

			header := []byte(fmt.Sprintf("# HELM UPGRADE: %s\n# Edit values below. Save & Close to Apply.\n# Comments starting with # will be ignored.\n\n", r.Name))
			finalContent := append(header, out...)

			tmpFile, err := os.CreateTemp("", fmt.Sprintf("helm-edit-%s-*.yaml", r.Name))
			if err != nil {
				return editorFinishedMsg{err: err}
			}
			if _, err := tmpFile.Write(finalContent); err != nil {
				return editorFinishedMsg{err: err}
			}
			tmpFile.Close()

			return helmValuesReadyMsg{filename: tmpFile.Name(), resource: r}

		case <-time.After(5 * time.Second):
			return editorFinishedMsg{err: fmt.Errorf("timeout fetching helm values")}
		}
	}
}


func repairChartDependencies(c *chart.Chart) {
	
	c.Lock = nil
	var cleanFiles []*chart.File
	for _, f := range c.Files {
		
		if strings.HasSuffix(f.Name, "Chart.lock") || strings.HasSuffix(f.Name, "requirements.lock") {
			continue
		}
		cleanFiles = append(cleanFiles, f)
	}
	c.Files = cleanFiles


	for _, dep := range c.Metadata.Dependencies {
		dep.Condition = "" 
		dep.Tags = nil     
		dep.Enabled = true 
	}


	for i, f := range c.Files {
		
		if f.Name == "Chart.yaml" || f.Name == "requirements.yaml" {
			
			var data map[string]interface{}
			if err := yaml.Unmarshal(f.Data, &data); err != nil {
				continue
			}

			
			if depsRaw, ok := data["dependencies"]; ok {
				if depsList, ok := depsRaw.([]interface{}); ok {
					var cleanDeps []interface{}
					changed := false

					for _, d := range depsList {
						if depMap, ok := d.(map[string]interface{}); ok {
							
							if _, exists := depMap["condition"]; exists {
								delete(depMap, "condition")
								changed = true
							}
							if _, exists := depMap["tags"]; exists {
								delete(depMap, "tags")
								changed = true
							}
							
							if val, exists := depMap["enabled"]; exists && val == false {
								delete(depMap, "enabled")
								changed = true
							}
							cleanDeps = append(cleanDeps, depMap)
						} else {
							cleanDeps = append(cleanDeps, d)
						}
					}

					
					if changed {
						data["dependencies"] = cleanDeps
						out, err := yaml.Marshal(data)
						if err == nil {
							c.Files[i].Data = out
						}
					}
				}
			}
		}
	}

	
	for _, subChart := range c.Dependencies() {
		repairChartDependencies(subChart)
	}
}

func (m *ExplorerModel) triggerDryRun(file string, r Resource) tea.Cmd {
	return func() tea.Msg {
		bytes, err := os.ReadFile(file)
		if err != nil {
			return upgradeCheckMsg{err: err}
		}

		var newVals map[string]interface{}
		if err := yaml.Unmarshal(bytes, &newVals); err != nil {
			return upgradeCheckMsg{err: fmt.Errorf("invalid YAML: %w", err)}
		}
		if newVals == nil {
			newVals = make(map[string]interface{})
		}

		os.Remove(file)

		cfg, err := m.k8sClient.NewHelmConfiguration(r.Namespace)
		if err != nil {
			return upgradeCheckMsg{err: err}
		}

		
		currentRel, err := action.NewGet(cfg).Run(r.Name)
		if err != nil {
			return upgradeCheckMsg{err: err}
		}

		
		if currentRel.Chart != nil {
			repairChartDependencies(currentRel.Chart)
			
			
			if err := chartutil.ProcessDependencies(currentRel.Chart, newVals); err != nil {
				return upgradeCheckMsg{err: fmt.Errorf("failed to process chart dependencies: %w", err)}
			}
		}

		upg := action.NewUpgrade(cfg)
		upg.Namespace = r.Namespace
		upg.DryRun = true
		upg.ReuseValues = false
		upg.ResetValues = true
		upg.DependencyUpdate = false 
		upg.Verify = false

		dryRel, err := upg.Run(r.Name, currentRel.Chart, newVals)
		if err != nil {
			return upgradeCheckMsg{err: fmt.Errorf("dry-run failed: %w", err)}
		}

		diff := utils.ComputeDiff(
			fmt.Sprintf("Current (v%d)", currentRel.Version), currentRel.Manifest,
			fmt.Sprintf("Proposed (Dry-Run)"), dryRel.Manifest,
			m.width,
		)

		currentObjs, _ := m.k8sClient.ParseManifestToObjects(currentRel.Manifest)
		newObjs, _ := m.k8sClient.ParseManifestToObjects(dryRel.Manifest)
		risks := helm.AnalyzeUpgradeRisks(currentObjs, newObjs)

		return upgradeCheckMsg{
			diff:    diff,
			newVals: newVals,
			risks:   risks,
		}
	}
}

func (m *ExplorerModel) performRealUpgrade(vals map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		type result struct {
			msg string
			err error
		}
		ch := make(chan result, 1)

		go func() {
			cfg, err := m.k8sClient.NewHelmConfiguration(m.activeEditResource.Namespace)
			if err != nil {
				ch <- result{err: err}
				return
			}

			
			rel, err := action.NewGet(cfg).Run(m.activeEditResource.Name)
			if err != nil {
				ch <- result{err: fmt.Errorf("failed to get release: %w", err)}
				return
			}

			
			if rel.Chart != nil {
				repairChartDependencies(rel.Chart)
				
				if vals == nil {
					vals = make(map[string]interface{})
				}

				if err := chartutil.ProcessDependencies(rel.Chart, vals); err != nil {
					ch <- result{err: fmt.Errorf("failed to process chart dependencies: %w", err)}
					return
				}
			}

			
			upg := action.NewUpgrade(cfg)
			upg.Namespace = m.activeEditResource.Namespace
			upg.ReuseValues = false
			upg.ResetValues = true
			upg.DependencyUpdate = false
			upg.Verify = false

			res, err := upg.Run(m.activeEditResource.Name, rel.Chart, vals)
			if err != nil {
				ch <- result{err: fmt.Errorf("upgrade failed: %w", err)}
				return
			}
			ch <- result{msg: fmt.Sprintf("Upgrade Successful: %s -> v%d", m.activeEditResource.Name, res.Version)}
		}()

		select {
		case res := <-ch:
			if res.err != nil {
				return helmFinishedMsg{err: res.err}
			}
			return helmFinishedMsg{msg: res.msg}
		case <-time.After(60 * time.Second):
			return helmFinishedMsg{err: fmt.Errorf("operation timed out (chart download slow?)")}
		}
	}
}

func (m *ExplorerModel) viewHelmDashboard() string {
	if m.helmDashboard == nil {
		return "Loading Dashboard..."
	}
	dashboardView := m.helmDashboard.View()
	footer := m.renderHelmFooter()
	return lipgloss.JoinVertical(lipgloss.Left, dashboardView, footer)
}

func (m *ExplorerModel) restartHelmWorkloads(r Resource) (tea.Model, tea.Cmd) {
	m.statusMsg = fmt.Sprintf("Restarting workloads for %s...", r.Name)
	return m, func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(r.Namespace)
		if err != nil {
			return helmFinishedMsg{err: err}
		}
		rel, err := action.NewGet(cfg).Run(r.Name)
		if err != nil {
			return helmFinishedMsg{err: err}
		}
		objs, _ := m.k8sClient.ParseManifestToObjects(rel.Manifest)
		count := 0
		for _, o := range objs {
			kind := o.GetKind()
			if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
				err := m.k8sClient.RestartResource(r.Namespace, kind, o.GetName())
				if err == nil {
					count++
				}
			}
		}
		return helmFinishedMsg{msg: fmt.Sprintf("Triggered restart on %d workloads.", count)}
	}
}

func (m *ExplorerModel) renderHelmFooterString() string {
	s := styles.ShortcutKeyStyle
	
	return fmt.Sprintf("%s Dash %s Vals %s SRE %s Hist %s Diff %s Drift %s Edit %s Uninst",
		s.Render("<Enter>"), s.Render("<v>"), s.Render("<H>"), s.Render("<?>"), s.Render("<C>"), s.Render("<W>"),
		s.Render("<e>"), s.Render("<X>"))
}


func (m *ExplorerModel) renderHelmFooter() string {
	helpStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#222222")).
		Foreground(lipgloss.Color("#888888"))

	helpText := " [TAB] Switch Tab | <Enter> Describe | <y> YAML | <e> Upgrade | <Esc> Close"
	footerBar := lipgloss.PlaceHorizontal(m.width, lipgloss.Left, helpStyle.Render(helpText),
		lipgloss.WithWhitespaceBackground(lipgloss.Color("#222222")))

	if m.statusMsg != "" {
		statusNotification := lipgloss.NewStyle().
			Background(lipgloss.Color("#FFA500")).
			Foreground(lipgloss.Color("#000000")).
			Width(m.width).
			Padding(0, 1).
			Render(m.statusMsg)
		
		
		return lipgloss.JoinVertical(lipgloss.Left, statusNotification, footerBar)
	}

	return footerBar
}

func (m *ExplorerModel) uninstallHelmRelease() tea.Cmd {
	if len(m.resources) == 0 {
		return nil
	}
	selected := m.resources[m.selectedIdx]
	return func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(selected.Namespace)
		if err != nil {
			return helmFinishedMsg{err: err}
		}

		un := action.NewUninstall(cfg)
		res, err := un.Run(selected.Name)
		if err != nil {
			return helmFinishedMsg{err: err}
		}
		return helmFinishedMsg{msg: fmt.Sprintf("Uninstalled %s (Info: %s)", selected.Name, res.Info)}
	}
}

func (m *ExplorerModel) rollbackHelmRelease() tea.Cmd {
	if len(m.resources) == 0 {
		return nil
	}
	return m.performRollback(0)
}

func (m *ExplorerModel) prepareRollbackDiff(r Resource) tea.Cmd {
	return func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(r.Namespace)
		if err != nil {
			return rollbackDiffMsg{err: err}
		}

		histClient := action.NewHistory(cfg)
		histClient.Max = 10
		history, err := histClient.Run(r.Name)
		if err != nil {
			return rollbackDiffMsg{err: fmt.Errorf("failed to fetch history: %w", err)}
		}

		sort.Slice(history, func(i, j int) bool {
			return history[i].Version > history[j].Version
		})

		if len(history) < 2 {
			return rollbackDiffMsg{err: fmt.Errorf("no previous revision found to rollback to")}
		}

		target := history[1]
		getClient := action.NewGet(cfg)

		fullCurrent, err := getClient.Run(r.Name)
		if err != nil {
			return rollbackDiffMsg{err: err}
		}

		getClient.Version = target.Version
		fullTarget, err := getClient.Run(r.Name)
		if err != nil {
			return rollbackDiffMsg{err: err}
		}

		diffString := utils.ComputeDiff(
			fmt.Sprintf("Current (v%d)", fullCurrent.Version), fullCurrent.Manifest,
			fmt.Sprintf("Target (v%d)", fullTarget.Version), fullTarget.Manifest,
			m.width,
		)

		return rollbackDiffMsg{
			content:   diffString,
			targetRev: target.Version,
		}
	}
}

func (m *ExplorerModel) performRollback(ver int) tea.Cmd {
	if len(m.resources) == 0 {
		return nil
	}
	selected := m.resources[m.selectedIdx]
	return func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(selected.Namespace)
		if err != nil {
			return helmFinishedMsg{err: err}
		}

		rb := action.NewRollback(cfg)
		rb.Version = ver 
		err = rb.Run(selected.Name)
		if err != nil {
			return helmFinishedMsg{err: err}
		}
		return helmFinishedMsg{msg: fmt.Sprintf("✅ Successfully rolled back %s to revision %d", selected.Name, ver)}
	}
}

func (m *ExplorerModel) prepareUninstall(r Resource) tea.Cmd {
	return func() tea.Msg {
		cfg, err := m.k8sClient.NewHelmConfiguration(r.Namespace)
		if err != nil {
			return blastRadiusMsg{err: err}
		}

		client := action.NewGet(cfg)
		rel, err := client.Run(r.Name)
		if err != nil {
			return blastRadiusMsg{err: fmt.Errorf("failed to fetch release for analysis: %w", err)}
		}

		objs, err := m.k8sClient.ParseManifestToObjects(rel.Manifest)
		if err != nil {
			return blastRadiusMsg{err: fmt.Errorf("failed to parse manifest: %w", err)}
		}

		report := helm.CalculateBlastRadius(objs)
		return blastRadiusMsg{report: report}
	}
}

func (m *ExplorerModel) initiateSafeAction(actionType string) (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]
	
	m.confirmMode = true
	m.confirmTarget = r.Name
	m.confirmInput.SetValue("")
	m.confirmInput.Focus()
	
	if actionType == "Uninstall" {
		m.pendingAction = func() tea.Cmd { return m.uninstallHelmRelease() }
		m.statusMsg = fmt.Sprintf("SAFE MODE: Type '%s' to confirm UNINSTALL", r.Name)
	} else if actionType == "Rollback" {
		m.pendingAction = func() tea.Cmd { return m.rollbackHelmRelease() }
		m.statusMsg = fmt.Sprintf("SAFE MODE: Type '%s' to confirm ROLLBACK", r.Name)
	}

	return m, textinput.Blink
}


func (m *ExplorerModel) triggerSREAnalysis() tea.Cmd {
	if len(m.resources) == 0 {
		return nil
	}
	selected := m.resources[m.selectedIdx]

	return func() tea.Msg {
		
		cfg, err := m.k8sClient.NewHelmConfiguration(selected.Namespace)
		if err != nil {
			return helmFinishedMsg{err: err}
		}

		
		client := action.NewHistory(cfg)
		client.Max = 10 
		hist, err := client.Run(selected.Name)
		if err != nil {
			return helmFinishedMsg{err: err}
		}

		
		sort.Slice(hist, func(i, j int) bool { return hist[i].Version > hist[j].Version })

		if len(hist) == 0 {
			return helmFinishedMsg{err: fmt.Errorf("release not found")}
		}

		
		objs, _ := m.k8sClient.ParseManifestToObjects(hist[0].Manifest)

		
		report := helm.AnalyzeReleaseHeuristics(hist, objs)

		return sreAnalysisReadyMsg{Report: report}
	}
}