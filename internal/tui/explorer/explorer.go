package explorer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"log"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amidipayan/kubevision/internal/k8s/auth"
	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/k8s/helm"
	"github.com/amidipayan/kubevision/internal/k8s/informer"
	"github.com/amidipayan/kubevision/internal/k8s/portforward"
	"github.com/amidipayan/kubevision/internal/tui/components/sre"
	"github.com/amidipayan/kubevision/internal/tui/components/tree"
	"github.com/amidipayan/kubevision/internal/tui/logs"
	"github.com/amidipayan/kubevision/internal/tui/styles"
	"github.com/amidipayan/kubevision/internal/tui/text"
	k8syamlviewer "github.com/amidipayan/kubevision/internal/tui/yaml"
	"github.com/amidipayan/kubevision/internal/utils"
	//k8ssre "github.com/amidipayan/kubevision/internal/k8s/sre"

	
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/kubernetes/scheme"
)



type triggerRepoSearchMsg struct {
	query string
}

type repoSearchResultsMsg struct {
	resources []Resource
	err       error
}

type repoIndexLoadedMsg struct {
	count int
	err   error
}

type streamReadyMsg struct {
	stream    io.ReadCloser
	title     string
	timeLabel string
	podName   string 
}

type textWindowMsg struct {
	title   string
	content string
	status  string
}

type helmFinishedMsg struct {
	err error
	msg string
}

type helmValuesReadyMsg struct {
	filename string
	resource Resource
}

type rollbackDiffMsg struct {
	content   string
	targetRev int
	err       error
}

type upgradeCheckMsg struct {
	diff    string
	newVals map[string]interface{}
	risks   helm.RiskAssessment
	err     error
}

type discoveryFinishedMsg struct {
	resources []schema.GroupVersionResource
	err       error
}

type connectionCheckMsg struct {
	err error
}


type statsResultMsg struct {
	cpu string
	mem string
	err error
}

type xRayReadyMsg struct {
	graph *tree.TopologyGraph
	err   error
}

type statsTickMsg struct{}
type metricsTickMsg struct{}
type editorFinishedMsg struct{ err error }
type XRayLoadedMsg struct{ Root *tree.Node }



type ExplorerModel struct {
	k8sClient          *client.KubeClient
	informerManager    *informer.Manager
	pfManager          *portforward.Manager
	views              map[string]ResourceView
	currentView        ResourceView
	rawResources       []Resource
	resources          []Resource
	selectedIdx        int
	tableStartIdx      int
	width, height      int
	quitting           bool
	namespace          string
	sortBy             string
	sortAsc            bool
	activeEditFile     string
	activeEditResource Resource
	diffBaseResource   *Resource
	diffMode           bool
	isOffline 		   bool
	podView            *PodView
	deployView         *DeploymentView
	serviceView        *ServiceView
	replicaSetView     *ReplicaSetView
	configMapView      *ConfigMapView
	secretView         *SecretView
	nodeView           *NodeView
	eventView          *EventView
	rbacView           *RBACView
	pvcView            *PVCView
	pvView             *PVView
	scView             *SCView
	jobView            *JobView
	cronJobView        *CronJobView
	helmView           *HelmView
	repoView           *RepoView
	showResourcePicker bool
	plugins            map[string][]Plugin
	
	

	
	textInput   textinput.Model
	inputMode   bool
	filterInput textinput.Model
	filterMode  bool
	cmdInput    textinput.Model
	cmdMode     bool

	
	showNamespaces     bool
	namespaceList      []string
	filteredNamespaces []string
	namespaceIdx       int

	
	showContexts bool
	contextList  []string
	contextIdx   int

	
	showContainers bool
	containerList  []string
	containerIdx   int
	targetLogPod   Resource

	pfInput         textinput.Model
	pfMode          bool
	viewingForwards bool

	
	pendingInstall bool

	
	yamlViewer *k8syamlviewer.YAMLViewer
	showYAML   bool
	textViewer *text.TextViewer
	showText   bool
	logViewer  *logs.LogViewer
	showLogs   bool
	xrayTree   tree.Model
	showXRay   bool

	
	helmDashboard *HelmDashboard
	showHelmDash  bool

	
	srePanel     *sre.Panel
	showSREPanel bool

	
	showRollbackDiff    bool
	rollbackDiffContent string
	rollbackTargetRev   int
	rollbackViewport    viewport.Model

	
	showUpgradeDiff    bool
	upgradeDiffContent string
	pendingUpgradeVals map[string]interface{}
	upgradeDryRunError bool
	upgradeRisks       helm.RiskAssessment

	
	confirmMode   bool
	confirmInput  textinput.Model
	confirmTarget string
	pendingAction func() tea.Cmd

	
	showBlastRadius bool
	blastReport     helm.BlastRadius

	statusMsg        string
	clusterInfo      client.ConfigInfo
	cpuUsage         string
	memUsage         string
	autoScrollEvents bool
	isDiscovering    bool

	
	showResources     bool     
	resourceList      []string 
	filteredResources []string 
	resourceIdx       int      
}

func NewExplorer(kc *client.KubeClient) *ExplorerModel {
	info := kc.GetConfigInfo()
	pfManager := portforward.NewManager(kc)

	ti := textinput.New()
	ti.Placeholder = "Namespace..."
	ti.CharLimit = 64
	ti.Width = 30
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))

	fi := textinput.New()
	fi.Placeholder = "Filter resources (e.g. status=Run)"
	fi.Prompt = "/"
	fi.CharLimit = 60
	fi.Width = 40
	fi.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))

	pfi := textinput.New()
	pfi.Placeholder = "8080:80"
	pfi.Prompt = "Forward: "
	pfi.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))

	ci := textinput.New()
	ci.Placeholder = "Command"
	ci.Prompt = ":"
	ci.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF"))

	// Confirmation Input
	confI := textinput.New()
	confI.Placeholder = "Type release name to confirm..."
	confI.CharLimit = 50
	confI.Width = 40
	confI.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)

	rbVp := viewport.New(100, 20)

	pluginMap, err := LoadPlugins()
    if err != nil {
        
        log.Printf("Warning: Could not load plugins: %v\n", err)
        pluginMap = make(map[string][]Plugin)
    }

	m := &ExplorerModel{
		k8sClient:        kc,
		plugins:          pluginMap,
		pfManager:        pfManager,
		namespace:        "default",
		sortBy:           "name",
		sortAsc:          true,
		textInput:        ti,
		filterInput:      fi,
		pfInput:          pfi,
		cmdInput:         ci,
		confirmInput:     confI,
		clusterInfo:      info,
		cpuUsage:         "...",
		memUsage:         "...",
		xrayTree:         tree.NewModel(100, 50),
		srePanel:         sre.NewPanel(100, 30),
		views:            make(map[string]ResourceView),
		autoScrollEvents: true,
		width:            100,
		height:           40,
		rollbackViewport: rbVp,
		isDiscovering:    true,
	}
	m.setupInformers()
	return m
}

func (m *ExplorerModel) setupInformers() {

	infManager := informer.NewManager(m.k8sClient)

	
	infManager.Watch(infManager.StaticFactory.Core().V1().Pods().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().Nodes().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().Services().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().ConfigMaps().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().Secrets().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().Events().Informer())
	infManager.Watch(infManager.StaticFactory.Apps().V1().Deployments().Informer())
	infManager.Watch(infManager.StaticFactory.Apps().V1().ReplicaSets().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().PersistentVolumes().Informer())
	infManager.Watch(infManager.StaticFactory.Core().V1().PersistentVolumeClaims().Informer())
	infManager.Watch(infManager.StaticFactory.Storage().V1().StorageClasses().Informer())
	infManager.Watch(infManager.StaticFactory.Batch().V1().Jobs().Informer())
	infManager.Watch(infManager.StaticFactory.Batch().V1().CronJobs().Informer())
	infManager.Watch(infManager.StaticFactory.Rbac().V1().RoleBindings().Informer())
	infManager.Watch(infManager.StaticFactory.Rbac().V1().ClusterRoleBindings().Informer())

	m.informerManager = infManager

	
	m.podView = NewPodView(infManager.StaticFactory.Core().V1().Pods().Lister())
	m.deployView = NewDeploymentView(infManager.StaticFactory.Apps().V1().Deployments().Lister())
	m.replicaSetView = NewReplicaSetView(infManager.StaticFactory.Apps().V1().ReplicaSets().Lister())
	m.jobView = NewJobView(infManager.StaticFactory.Batch().V1().Jobs().Lister())
	m.cronJobView = NewCronJobView(infManager.StaticFactory.Batch().V1().CronJobs().Lister())
	m.nodeView = NewNodeView(infManager.StaticFactory.Core().V1().Nodes().Lister())
	m.serviceView = NewServiceView(m.k8sClient)
	m.configMapView = NewConfigMapView(infManager.StaticFactory.Core().V1().ConfigMaps().Lister())
	m.secretView = NewSecretView(infManager.StaticFactory.Core().V1().Secrets().Lister())
	m.eventView = NewEventView(infManager.StaticFactory)
	m.pvcView = NewPVCView(infManager.StaticFactory.Core().V1().PersistentVolumeClaims().Lister())
	m.pvView = NewPVView(infManager.StaticFactory.Core().V1().PersistentVolumes().Lister())
	m.scView = NewSCView(infManager.StaticFactory.Storage().V1().StorageClasses().Lister())
	m.rbacView = NewRBACView(infManager.StaticFactory)
	m.helmView = NewHelmView(m.k8sClient)
	m.repoView = NewRepoView()

	
	m.views = make(map[string]ResourceView)
	m.views["pods"] = m.podView
	m.views["deployments"] = m.deployView
	m.views["replicasets"] = m.replicaSetView
	m.views["jobs"] = m.jobView
	m.views["cronjobs"] = m.cronJobView
	m.views["nodes"] = m.nodeView
	m.views["services"] = m.serviceView
	m.views["configmaps"] = m.configMapView
	m.views["secrets"] = m.secretView
	m.views["events"] = m.eventView
	m.views["pvc"] = m.pvcView
	m.views["pv"] = m.pvView
	m.views["storageclasses"] = m.scView
	m.views["rbac"] = m.rbacView
	m.views["helm"] = m.helmView
	m.views["charts"] = m.repoView

	
	currentTitle := "Pods"
	if m.currentView != nil {
		currentTitle = m.currentView.Title()
	}

	found := false
	for _, v := range m.views {
		if v.Title() == currentTitle {
			m.currentView = v
			found = true
			break
		}
	}
	if !found {
		m.currentView = m.views["pods"]
	}

	
	go infManager.Start()
}

func (m *ExplorerModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		textinput.Blink,
		m.waitForK8sUpdates(),
		m.performDiscovery(),
		m.tickStats(),
		m.tickMetrics(),
		m.checkConnection(),
	)
}


func (m *ExplorerModel) loadRepoIndex() tea.Cmd {
	return func() tea.Msg {
		if repoView, ok := m.views["charts"].(*RepoView); ok {
			count, err := repoView.LoadIndex()
			return repoIndexLoadedMsg{count: count, err: err}
		}
		return nil
	}
}

func (m *ExplorerModel) debounceRepoSearch(query string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
		return triggerRepoSearchMsg{query: query}
	})
}

func (m *ExplorerModel) performAsyncRepoSearch(query string) tea.Cmd {
	return func() tea.Msg {
		if view, ok := m.views["charts"]; ok {
			res, err := view.Retrieve(query)
			return repoSearchResultsMsg{resources: res, err: err}
		}
		return repoSearchResultsMsg{err: fmt.Errorf("charts view not found")}
	}
}


func (m *ExplorerModel) fetchNamespaces() {
	nsList, err := m.k8sClient.GetClientset().CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to list namespaces: %v", err)
		return
	}

	var names []string
	names = append(names, "ALL NAMESPACES") 

	for _, n := range nsList.Items {
		names = append(names, n.Name)
	}
	sort.Strings(names[1:]) 

	m.namespaceList = names
	m.filteredNamespaces = names
	m.namespaceIdx = 0

	
	m.filterInput.SetValue("")
	m.filterInput.Placeholder = "Filter namespaces..."
	m.filterInput.Focus()
}

func (m *ExplorerModel) filterNamespaceList() {
	term := strings.ToLower(m.filterInput.Value())
	if term == "" {
		m.filteredNamespaces = m.namespaceList
		return
	}

	var matches []string
	for _, ns := range m.namespaceList {
		if strings.Contains(strings.ToLower(ns), term) {
			matches = append(matches, ns)
		}
	}
	m.filteredNamespaces = matches
	m.namespaceIdx = 0
}


func (m *ExplorerModel) promptInstallChart() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}

	chart := m.resources[m.selectedIdx]
	m.cmdMode = true
	m.cmdInput.Placeholder = "Release Name"
	m.cmdInput.SetValue(chart.Name) 
	m.cmdInput.Focus()
	m.statusMsg = fmt.Sprintf("INSTALL: Enter Release Name for '%s/%s'", chart.Extras[0], chart.Name)
	m.pendingInstall = true

	return m, textinput.Blink
}

func (m *ExplorerModel) installSelectedChart(releaseName string) tea.Cmd {
	return func() tea.Msg {
		if len(m.resources) == 0 {
			return nil
		}
		chart := m.resources[m.selectedIdx]
		repoName := chart.Extras[0] 
		fullChartName := fmt.Sprintf("%s/%s", repoName, chart.Name)

		cmd := exec.Command("helm", "install", releaseName, fullChartName, "-n", m.namespace)
		out, err := cmd.CombinedOutput()

		if err != nil {
			return helmFinishedMsg{err: fmt.Errorf("Install Failed: %v\n%s", err, string(out))}
		}
		return helmFinishedMsg{msg: fmt.Sprintf("🚀 Successfully installed release '%s' (%s)", releaseName, fullChartName)}
	}
}

func (m *ExplorerModel) performDiscovery() tea.Cmd {
	return func() tea.Msg {
		res, err := m.k8sClient.DiscoverResources()
		return discoveryFinishedMsg{resources: res, err: err}
	}
}

func (m *ExplorerModel) waitForK8sUpdates() tea.Cmd {
	return func() tea.Msg { return m.informerManager.WaitForUpdates() }
}

func (m *ExplorerModel) tickStats() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return statsTickMsg{} })
}
func (m *ExplorerModel) tickMetrics() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return metricsTickMsg{} })
}



func (m *ExplorerModel) fetchClusterStats() tea.Cmd {
	return func() tea.Msg {
		
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		
		type result struct {
			cpu string
			mem string
			err error
		}
		
		
		ch := make(chan result, 1)

		
		go func() {

			cpu, mem, err := m.k8sClient.GetClusterStats()
			ch <- result{cpu: cpu, mem: mem, err: err}
		}()

	
		select {
		case res := <-ch:
			
			return statsResultMsg{cpu: res.cpu, mem: res.mem, err: res.err}
		case <-ctx.Done():
			
			return statsResultMsg{err: fmt.Errorf("timeout fetching metrics")}
		}
	}
}

func (m *ExplorerModel) fetchPodMetrics() {
	podView, ok := m.views["pods"].(*PodView)
	if !ok {
		return
	}
	mc := m.k8sClient.GetMetricsClient()
	if mc == nil {
		return
	}
	list, err := mc.MetricsV1beta1().PodMetricses(m.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	data := make(map[string]PodMetricData)
	podLister := m.informerManager.StaticFactory.Core().V1().Pods().Lister()

	for _, i := range list.Items {
		var c, mem int64
		for _, cn := range i.Containers {
			c += cn.Usage.Cpu().MilliValue()
			mem += cn.Usage.Memory().Value()
		}
		var limitCPU, limitMem int64
		pod, err := podLister.Pods(i.Namespace).Get(i.Name)
		if err == nil {
			for _, container := range pod.Spec.Containers {
				limitCPU += container.Resources.Limits.Cpu().MilliValue()
				limitMem += container.Resources.Limits.Memory().Value()
			}
		}
		cpuStr := m.renderUsageBar(c, limitCPU, "cpu")
		memStr := m.renderUsageBar(mem, limitMem, "mem")

		data[i.Namespace+"/"+i.Name] = PodMetricData{
			CPURaw: c,
			MemRaw: mem,
			CPU:    cpuStr,
			Memory: memStr,
		}
	}
	podView.SetMetrics(data)
}

func (m *ExplorerModel) renderUsageBar(usage, limit int64, resourceType string) string {
	if limit <= 0 {
		if resourceType == "cpu" {
			return fmt.Sprintf("%dm", usage)
		}
		return fmt.Sprintf("%dMi", usage/1024/1024)
	}

	percent := float64(usage) / float64(limit)
	if percent > 1.0 {
		percent = 1.0
	}

	width := 8
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	fillChar := "█"
	emptyChar := "░"

	var style lipgloss.Style
	if percent < 0.8 {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")) 
	} else if percent < 0.9 {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")) 
	} else {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")) 
	}

	bar := style.Render(strings.Repeat(fillChar, filled)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render(strings.Repeat(emptyChar, empty))

	percentageStr := fmt.Sprintf("%d%%", int(percent*100))
	return fmt.Sprintf("%s %s", bar, percentageStr)
}

func (m *ExplorerModel) fetchNodeMetrics() {
	nodeView, ok := m.views["nodes"].(*NodeView)
	if !ok {
		return
	}
	mc := m.k8sClient.GetMetricsClient()
	if mc == nil {
		return
	}
	list, err := mc.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}
	data := make(map[string]NodeMetricData)
	for _, i := range list.Items {
		c := i.Usage.Cpu().MilliValue()
		mem := i.Usage.Memory().Value()
		data[i.Name] = NodeMetricData{
			CPURaw: c,
			MemRaw: mem,
			CPU:    fmt.Sprintf("%dm", c),
			Memory: fmt.Sprintf("%dMi", mem/1024/1024),
		}
	}
	nodeView.SetMetrics(data)
}


func (m *ExplorerModel) performFilter() {
	if m.currentView.Title() == "Helm Charts" {
		return
	}

	term := strings.TrimSpace(m.filterInput.Value())

	if term == "" {
		m.resources = make([]Resource, len(m.rawResources))
		copy(m.resources, m.rawResources)
		m.sortResources()
		m.clampCursor()
		m.syncTableScroll()
		return
	}

	var filtered []Resource
	isSmartQuery := strings.Contains(term, "=")

	for _, r := range m.rawResources {
		if isSmartQuery {
			if m.matchSmartFilter(r, term) {
				filtered = append(filtered, r)
			}
		} else {
			if m.matchFuzzy(r, term) {
				filtered = append(filtered, r)
			}
		}
	}

	m.resources = filtered
	m.sortResources()
	m.clampCursor()
	m.syncTableScroll()
}

var semanticAliases = map[string][]string{
	
	"failed":  {"Error", "CrashLoopBackOff", "Evicted", "OOMKilled", "ImagePullBackOff", "CreateContainerConfigError", "ErrImagePull", "Terminating", "NodeNotReady"},
	"bad":     {"Error", "CrashLoopBackOff", "Evicted", "OOMKilled", "ImagePullBackOff", "Unknown", "Missing", "Warning", "Failed"},
	"stuck":   {"Pending", "ContainerCreating", "Terminating", "Init", "PodInitializing", "Waiting", "CrashLoopBackOff"},
	"healthy": {"Running", "Completed", "Succeeded", "Ready", "Bound", "Active"},
    "warn":    {"Warning", "Failed", "Error"}, 

	
	"db":       {"postgres", "mysql", "redis", "mongo", "mariadb", "memcached", "rabbitmq", "cockroach", "cassandra", "elasticsearch", "statefulset", "pvc"},
	"data":     {"postgres", "mysql", "redis", "mongo", "mariadb", "statefulset", "pvc", "pv", "storageclass"},
	"web":      {"nginx", "apache", "httpd", "tomcat", "caddy", "traefik", "ingress", "loadbalancer"},
	"frontend": {"react", "vue", "angular", "ui", "web", "nginx", "client"},
	"backend":  {"api", "server", "worker", "go", "java", "python", "node", "service"},
	"cron":     {"cronjob", "batch", "job", "schedule"},
    
    
    "deploy":   {"Deployment", "ReplicaSet", "ReplicationController"},
    "ds":       {"DaemonSet"},
    "sts":      {"StatefulSet"},
    "app":      {"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service"},

	
	"public":   {"LoadBalancer", "NodePort", "Ingress", "ExternalName", "Gateway"}, 
	"external": {"LoadBalancer", "NodePort", "Ingress"},
	"net":      {"Service", "Ingress", "NetworkPolicy", "Endpoint", "Route", "Gateway"},

	
	"sec":    {"Secret", "ServiceAccount", "Role", "RoleBinding", "ClusterRole", "Opaque", "TLS"},
	"auth":   {"Secret", "ServiceAccount", "Role", "RoleBinding", "Token", "User"},
	"admin":  {"cluster-admin", "root", "master", "system"},
	"config": {"ConfigMap", "Secret", "Env"},

	
	"disk":    {"PersistentVolume", "PersistentVolumeClaim", "StorageClass", "NFS", "CSI", "VolumeSnapshot"}, 
	"storage": {"PersistentVolume", "PersistentVolumeClaim", "StorageClass", "VolumeSnapshot"},

    
    "scale":   {"HorizontalPodAutoscaler", "VerticalPodAutoscaler", "Replicas", "HPA", "VPA"},
    "limit":   {"ResourceQuota", "LimitRange", "Quota"},
    "quota":   {"ResourceQuota", "ClusterResourceQuota"},

    
    "node":    {"Node", "Machine", "MachineSet"},
    "cluster": {"Node", "Namespace", "ComponentStatus", "Lease", "Event"},
    "ns":      {"Namespace"},
    "crd":     {"CustomResourceDefinition"},
    
    
    "policy":  {"NetworkPolicy", "PodDisruptionBudget", "ValidatingWebhookConfiguration", "MutatingWebhookConfiguration", "LimitRange"},
    "pdb":     {"PodDisruptionBudget"},
}


func (m *ExplorerModel) matchFuzzy(r Resource, term string) bool {
	termLower := strings.ToLower(term)

	
	if aliases, ok := semanticAliases[termLower]; ok {
	
		for _, alias := range aliases {
			if m.rawMatch(r, alias) {
				return true
			}
		}

	}

	
	return m.rawMatch(r, term)
}


func (m *ExplorerModel) rawMatch(r Resource, term string) bool {
	term = strings.ToLower(term)

	
	if strings.Contains(strings.ToLower(r.Name), term) || strings.Contains(strings.ToLower(r.Namespace), term) {
		return true
	}

	
	if strings.Contains(strings.ToLower(r.Kind), term) || strings.Contains(strings.ToLower(r.Status), term) {
		return true
	}


	for _, extra := range r.Extras {
		if strings.Contains(strings.ToLower(extra), term) {
			return true
		}
	}

	return false
}

func (m *ExplorerModel) matchSmartFilter(r Resource, query string) bool {
	parts := strings.Fields(query)
	for _, part := range parts {
		if !strings.Contains(part, "=") {
			if !strings.Contains(strings.ToLower(r.Name), strings.ToLower(part)) {
				return false
			}
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) < 2 {
			continue
		}
		key := strings.ToLower(kv[0])
		val := strings.ToLower(kv[1])

		matched := false
		switch key {
		case "n", "name":
			matched = strings.Contains(strings.ToLower(r.Name), val)
		case "ns", "namespace":
			matched = strings.Contains(strings.ToLower(r.Namespace), val)
		case "k", "kind":
			matched = strings.Contains(strings.ToLower(r.Kind), val)
		case "s", "status":
			matched = strings.Contains(strings.ToLower(r.Status), val)
		default:
			matched = false
		}
		if !matched {
			return false
		}
	}
	return true
}

func (m *ExplorerModel) sortResources() {
	if m.currentView.Title() == "Events" && m.sortBy == "age" {
		return
	}
	sort.Slice(m.resources, func(i, j int) bool {
		var less bool
		switch m.sortBy {
		case "age":
			less = m.resources[i].AgeRaw.Before(m.resources[j].AgeRaw)
		case "restarts":
			less = m.resources[i].Restarts < m.resources[j].Restarts
		case "cpu":
			less = m.resources[i].CPURaw < m.resources[j].CPURaw
		case "mem":
			less = m.resources[i].MemoryRaw < m.resources[j].MemoryRaw
		default:
			less = m.resources[i].Name < m.resources[j].Name
		}
		if m.sortAsc {
			return less
		}
		return !less
	})
}

func (m *ExplorerModel) setSort(key string) {
	if m.sortBy == key {
		m.sortAsc = !m.sortAsc
	} else {
		m.sortBy = key
		m.sortAsc = false
		if key == "name" || key == "age" {
			m.sortAsc = true
		}
	}
	m.performFilter()
}

func (m *ExplorerModel) clampCursor() {
	if len(m.resources) == 0 {
		m.selectedIdx = 0
		return
	}
	if m.selectedIdx >= len(m.resources) {
		m.selectedIdx = len(m.resources) - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

func (m *ExplorerModel) syncTableScroll() {
	visible := m.height - 15
	if visible < 1 {
		visible = 1
	}
	if m.selectedIdx < m.tableStartIdx {
		m.tableStartIdx = m.selectedIdx
	} else if m.selectedIdx >= m.tableStartIdx+visible {
		m.tableStartIdx = m.selectedIdx - visible + 1
	}
	maxStart := len(m.resources) - visible
	if maxStart < 0 {
		maxStart = 0
	}
	if m.tableStartIdx > maxStart {
		m.tableStartIdx = maxStart
	}
	if m.tableStartIdx < 0 {
		m.tableStartIdx = 0
	}
}

func (m *ExplorerModel) fetchInternalYAML(r Resource) (string, error) {
	
	clientset := m.k8sClient.GetClientset()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var obj k8sruntime.Object
	var err error
	var gvk schema.GroupVersionKind

	switch r.Kind {
	case "Pod":
		obj, err = clientset.CoreV1().Pods(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	case "Service":
		obj, err = clientset.CoreV1().Services(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	case "Deployment":
		obj, err = clientset.AppsV1().Deployments(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	case "ReplicaSet":
		obj, err = clientset.AppsV1().ReplicaSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}
	case "ConfigMap":
		obj, err = clientset.CoreV1().ConfigMaps(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	case "Secret":
		obj, err = clientset.CoreV1().Secrets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	case "Node":
		obj, err = clientset.CoreV1().Nodes().Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}
	case "Event":
		obj, err = clientset.CoreV1().Events(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Event"}
	case "PersistentVolumeClaim":
		obj, err = clientset.CoreV1().PersistentVolumeClaims(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
	case "PersistentVolume":
		obj, err = clientset.CoreV1().PersistentVolumes().Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"}
	case "StorageClass":
		obj, err = clientset.StorageV1().StorageClasses().Get(ctx, r.Name, metav1.GetOptions{})
		gvk = schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}
	default:
		return m.fetchGenericYAML(r)
	}

	if err != nil {
		return "", err
	}
	if !gvk.Empty() {
		obj.GetObjectKind().SetGroupVersionKind(gvk)
	} else {
		gvks, _, _ := scheme.Scheme.ObjectKinds(obj)
		if len(gvks) > 0 {
			obj.GetObjectKind().SetGroupVersionKind(gvks[0])
		}
	}
	if accessor, err := meta.Accessor(obj); err == nil {
		accessor.SetManagedFields(nil)
	}
	printer := printers.YAMLPrinter{}
	var buf bytes.Buffer
	if err := printer.PrintObj(obj, &buf); err != nil {
		return "", fmt.Errorf("failed to print YAML: %w", err)
	}
	return buf.String(), nil
}

func (m *ExplorerModel) fetchGenericYAML(r Resource) (string, error) {
	cmd := exec.Command("kubectl", "get", r.Kind, r.Name, "-n", r.Namespace, "-o", "yaml")
	if r.Namespace == "" {
		cmd = exec.Command("kubectl", "get", r.Kind, r.Name, "-o", "yaml")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v", string(out))
	}
	return string(out), nil
}

func (m *ExplorerModel) copyToClipboard(content string) {
	utils.CopyToClipboard(content)
	m.statusMsg = "Copied to clipboard!"
}



func (m *ExplorerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    
	
	if m.showLogs && m.logViewer != nil {
		var cmd tea.Cmd
		newModel, newCmd := m.logViewer.Update(msg)
		m.logViewer = newModel.(*logs.LogViewer)
		cmd = newCmd
		if _, ok := msg.(logs.CloseLogViewMsg); ok {
			m.showLogs = false
			m.logViewer = nil
			return m, nil
		}
		return m, cmd
	}

	if m.showXRay {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "esc" || key.String() == "q" {
				m.showXRay = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.xrayTree, cmd = m.xrayTree.Update(msg)
		return m, cmd
	}

	if m.showSREPanel {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "esc" || key.String() == "?" || key.String() == "q" {
				m.showSREPanel = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		newModel, cmd := m.srePanel.Update(msg)
		m.srePanel = newModel.(*sre.Panel)
		return m, cmd
	}

	if m.showResources {
		return m.updateResourcePicker(msg)
	}

	if m.showYAML && m.yamlViewer != nil {
		var cmd tea.Cmd
		newModel, newCmd := m.yamlViewer.Update(msg)
		m.yamlViewer = newModel.(*k8syamlviewer.YAMLViewer)
		cmd = newCmd
		if _, ok := msg.(k8syamlviewer.ReturnToExplorerMsg); ok {
			m.showYAML = false
			m.yamlViewer = nil
			m.refreshData()
			return m, nil
		}
		return m, cmd
	}

	if m.showText && m.textViewer != nil {
		var cmd tea.Cmd
		newModel, newCmd := m.textViewer.Update(msg)
		m.textViewer = newModel.(*text.TextViewer)
		cmd = newCmd
		if _, ok := msg.(text.CloseTextViewMsg); ok {
			m.showText = false
			m.textViewer = nil
			m.statusMsg = "Returned to Explorer"
			return m, nil
		}
		return m, cmd
	}

	
	if m.showNamespaces {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.showNamespaces = false
				m.statusMsg = "Cancelled namespace switch"
				return m, nil
			case "up", "k":
				if m.namespaceIdx > 0 {
					m.namespaceIdx--
				}
			case "down", "j":
				if m.namespaceIdx < len(m.filteredNamespaces)-1 {
					m.namespaceIdx++
				}
			case "enter":
				if len(m.filteredNamespaces) > 0 {
					selection := m.filteredNamespaces[m.namespaceIdx]
					if selection == "ALL NAMESPACES" {
						m.namespace = ""
					} else {
						m.namespace = selection
					}
					m.showNamespaces = false
					m.selectedIdx = 0
					m.tableStartIdx = 0
					m.statusMsg = fmt.Sprintf("Switched to namespace: %s", selection)
					m.refreshData()
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filterNamespaceList()
		return m, cmd
	}

	if m.showRollbackDiff {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y", "enter":
				m.showRollbackDiff = false
				m.statusMsg = fmt.Sprintf("🚀 Initiating Rollback to Revision %d...", m.rollbackTargetRev)
				return m, m.performRollback(m.rollbackTargetRev)
			case "n", "N", "q", "esc":
				m.showRollbackDiff = false
				m.statusMsg = "Rollback cancelled."
				return m, nil
			default:
				var cmd tea.Cmd
				m.rollbackViewport, cmd = m.rollbackViewport.Update(msg)
				return m, cmd
			}
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.rollbackViewport.Width = msg.Width
			m.rollbackViewport.Height = msg.Height - 6
		}
		return m, nil
	}

	if m.showUpgradeDiff {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y", "enter":
				m.showUpgradeDiff = false
				m.statusMsg = "🚀 Applying Upgrade..."
				return m, m.performRealUpgrade(m.pendingUpgradeVals)
			case "n", "N", "q", "esc":
				m.showUpgradeDiff = false
				m.upgradeDryRunError = false
				m.statusMsg = "Upgrade cancelled."
				return m, nil
			default:
				var cmd tea.Cmd
				m.rollbackViewport, cmd = m.rollbackViewport.Update(msg)
				return m, cmd
			}
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.rollbackViewport.Width = msg.Width
			m.rollbackViewport.Height = msg.Height - 6
		}
		return m, nil
	}

	switch msg := msg.(type) {

	
	case triggerRepoSearchMsg:

		if msg.query == "__LOAD_INDEX__" {
             return m, m.loadRepoIndex()
        }

		if m.currentView.Title() == "Helm Charts" && msg.query == m.filterInput.Value() {
			return m, m.performAsyncRepoSearch(msg.query)
		}
		return m, nil
	
	case connectionCheckMsg:
		var cmd tea.Cmd
		if msg.err != nil {

			m.isOffline = true
			m.statusMsg = fmt.Sprintf("⚠️  LOST CONNECTION: %v", msg.err)
			
			cmd = m.checkConnection()
		} else {
			
			if m.isOffline {
				m.isOffline = false
				m.statusMsg = "✅ Connection Restored! Reloading informers..."
				if m.informerManager != nil {
					m.informerManager.Stop()
				}
				m.setupInformers()
				m.refreshData()
			}
			cmd = m.checkConnection()
		}
		return m, cmd

	
	case repoIndexLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Index Load Failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Helm Index Loaded: %d charts ready.", msg.count)
			if m.currentView.Title() == "Helm Charts" {
				return m, m.performAsyncRepoSearch("")
			}
		}
		return m, nil

	case repoSearchResultsMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Search Error: %v", msg.err)
		} else {
			m.rawResources = msg.resources
			m.resources = msg.resources
			m.clampCursor()
			m.syncTableScroll()
			m.statusMsg = fmt.Sprintf("Found %d charts", len(m.resources))
		}
		return m, nil

	case discoveryFinishedMsg:
		m.isDiscovering = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Discovery Warning: %v", msg.err)
		} else {
			count := 0
			for _, gvr := range msg.resources {
				key := strings.ToLower(gvr.Resource)
				if _, exists := m.views[key]; !exists {
					m.views[key] = NewGenericView(m.k8sClient, gvr, gvr.Resource)
					count++
				}
			}
			m.statusMsg = fmt.Sprintf("Discovered %d new resource types.", count)
		}
		m.populateResourceList()
		return m, nil

	
	case textWindowMsg:
		m.textViewer = text.NewTextViewer(
			msg.title,
			msg.content,
			m.width, m.height,
		)
		m.showText = true
		m.diffMode = false
		m.statusMsg = msg.status
		return m, nil

	case helmFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Helm Error: %v", msg.err)
		} else {
			m.statusMsg = msg.msg
		}
		m.refreshData()
		return m, nil
	

	case streamReadyMsg:
		m.logViewer = logs.NewLogViewer(msg.title, msg.podName, m.width, m.height, msg.stream)
		if msg.timeLabel != "" {
			m.logViewer.SetTimeFilter(msg.timeLabel)
		}
		m.showLogs = true
		m.statusMsg = "Streaming logs..."
		return m, m.logViewer.Init()

	case blastRadiusMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Analysis Failed: %v", msg.err)
			m.confirmMode = true
			if len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				m.confirmTarget = r.Name
				m.confirmInput.SetValue("")
				m.confirmInput.Focus()
				m.pendingAction = func() tea.Cmd { return m.uninstallHelmRelease() }
				m.statusMsg = fmt.Sprintf("SAFE MODE: Type '%s' to confirm UNINSTALL", r.Name)
			}
			return m, textinput.Blink
		}
		m.blastReport = msg.report
		m.showBlastRadius = true

		m.confirmMode = true
		r := m.resources[m.selectedIdx]
		m.confirmTarget = r.Name
		m.confirmInput.SetValue("")
		m.confirmInput.Focus()
		m.pendingAction = func() tea.Cmd { return m.uninstallHelmRelease() }
		return m, textinput.Blink

	case sreAnalysisReadyMsg:
		m.srePanel.SetAnalysis(msg.Report)
		m.srePanel.Width = m.width - 10
		m.srePanel.Height = m.height - 6
		m.srePanel.Active = true
		m.showSREPanel = true
		m.statusMsg = "SRE Analysis Complete."
		return m, nil

	case helmValuesReadyMsg:
		m.activeEditFile = msg.filename
		m.activeEditResource = msg.resource
		c := utils.BuildEditorCmd(m.activeEditFile)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return editorFinishedMsg{err: err}
		})

	case editorFinishedMsg:
	if msg.err != nil {
		m.statusMsg = fmt.Sprintf("Editor Error: %v", msg.err)
	} else {
		if m.activeEditResource.Kind == "HelmRelease" {
			m.statusMsg = "Calculating Upgrade Diff (Dry Run)..."
			return m, m.triggerDryRun(m.activeEditFile, m.activeEditResource)
		} else {
			editedBytes, err := os.ReadFile(m.activeEditFile)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Failed to read edited file: %v", err)
			} else {
				decoder := k8syamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
				obj := &unstructured.Unstructured{}
				_, _, decodeErr := decoder.Decode(editedBytes, nil, obj)
				
				err = m.k8sClient.ApplyYAML(m.namespace, editedBytes)
				if err != nil {
					m.statusMsg = fmt.Sprintf("Apply Failed: %v", err)
				} else {
					m.statusMsg = "Resource updated successfully!"
					if decodeErr == nil {
						newName := obj.GetName()
						newNs := obj.GetNamespace()
						
						oldName := m.activeEditResource.Name
						oldNs := m.activeEditResource.Namespace
						
						gvr := m.currentView.GetGVR()
						
						
						isClusterScoped := gvr.Resource == "nodes" || 
										   gvr.Resource == "persistentvolumes" || 
										   gvr.Resource == "storageclasses" || 
										   gvr.Resource == "namespaces" ||
										   gvr.Resource == "clusterroles" ||
										   gvr.Resource == "clusterrolebindings"

						if isClusterScoped {
						
							newNs = ""
							oldNs = ""
						} else if newNs == "" {
							newNs = "default"
						}
					
						if (newName != "" && newName != oldName) || (newNs != "" && newNs != oldNs) {
							m.statusMsg = fmt.Sprintf("Renamed: %s -> %s", oldName, newName)
							
							delErr := m.k8sClient.GetDynamicClient().Resource(gvr).Namespace(oldNs).Delete(context.TODO(), oldName, metav1.DeleteOptions{})
							if delErr != nil {
								exec.Command("kubectl", "delete", gvr.Resource, oldName, "-n", oldNs).Run()
							}
						}
					}
				}
			}
			os.Remove(m.activeEditFile)
			m.activeEditFile = ""
		}
		m.refreshData()
	}
	return m, tea.ClearScreen

	case upgradeCheckMsg:
		if msg.err != nil {
			m.upgradeDryRunError = true
			errorContent := fmt.Sprintf("\n⚠️  DRY RUN FAILED ⚠️\n\n%v\n\n", msg.err)
			errorContent += lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(
				"NOTE: This often happens if the chart depends on library charts (e.g. common) \n" +
					"that are not fully hydrated in the release history.\n\n" +
					"You can likely proceed if your values are correct.\n")
			m.upgradeDiffContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(errorContent)
		} else {
			m.upgradeDryRunError = false
			m.upgradeDiffContent = msg.diff
			m.upgradeRisks = msg.risks
		}

		m.pendingUpgradeVals = msg.newVals
		m.showUpgradeDiff = true
		headerH := 3
		footerH := 3
		vpHeight := m.height - headerH - footerH
		riskSpace := 0
		if len(m.upgradeRisks.Factors) > 0 {
			riskSpace = len(m.upgradeRisks.Factors) + 5
		}
		vpHeight -= riskSpace
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.rollbackViewport = viewport.New(m.width, vpHeight)
		m.rollbackViewport.SetContent(m.upgradeDiffContent)
		return m, nil
	
	case ShowServiceAnalysisMsg:
		if m.srePanel == nil {
			m.srePanel = sre.NewPanel(m.width, m.height)
		}

		var mappedResults []helm.HeuristicResult
		
		
		getRemediation := func(risk string) string {
			risk = strings.ToLower(risk)
			switch {
			case strings.Contains(risk, "single point of failure"), strings.Contains(risk, "replica"):
				return "Action: kubectl scale deployment <name> --replicas=3 (Min 2 required for HA)"
			case strings.Contains(risk, "affinity"), strings.Contains(risk, "co-location"):
				return "Fix: Add 'podAntiAffinity' to spec.affinity to spread pods across distinct nodes."
			case strings.Contains(risk, "pdb"), strings.Contains(risk, "disruption"):
				return "Action: kubectl create pdb <name>-pdb --selector=app=<label> --min-available=1"
			case strings.Contains(risk, "strategy"), strings.Contains(risk, "recreate"):
				return "Fix: Change deployment strategy to 'RollingUpdate' with maxUnavailable=25%."

			
			case strings.Contains(risk, "privilege"), strings.Contains(risk, "root"), strings.Contains(risk, "uid 0"):
				return "Fix: Set 'securityContext.runAsNonRoot: true' and 'runAsUser: 1000+'."
			case strings.Contains(risk, "capabilities"), strings.Contains(risk, "cap_sys_admin"):
				return "Fix: Add 'securityContext.capabilities.drop: [\"ALL\"]' to container spec."
			case strings.Contains(risk, "filesystem"), strings.Contains(risk, "immutable"):
				return "Fix: Set 'securityContext.readOnlyRootFilesystem: true' to prevent tampering."
			case strings.Contains(risk, "serviceaccount"), strings.Contains(risk, "token"):
				return "Fix: Set 'automountServiceAccountToken: false' if API access is not required."
			case strings.Contains(risk, "networkpolicy"), strings.Contains(risk, "zero trust"), strings.Contains(risk, "lateral"):
				return "Action: Apply a 'Default Deny' NetworkPolicy to isolate this namespace."
			case strings.Contains(risk, "host"), strings.Contains(risk, "hostnetwork"):
				return "Critical: Remove 'hostNetwork: true' or 'hostPath' mounts to prevent node takeover."


			case strings.Contains(risk, "liveness"), strings.Contains(risk, "deadlock"):
				return "Fix: Add 'livenessProbe' (exec/httpGet) to restart stuck containers automatically."
			case strings.Contains(risk, "readiness"), strings.Contains(risk, "traffic"):
				return "Fix: Add 'readinessProbe' to prevent traffic being sent to unready pods."
			case strings.Contains(risk, "startup"), strings.Contains(risk, "slow start"):
				return "Fix: Add 'startupProbe' to allow slow initialization without crash loops."

			
			case strings.Contains(risk, "limit"), strings.Contains(risk, "request"), strings.Contains(risk, "qos"), strings.Contains(risk, "starvation"):
				return "Fix: Define 'resources.requests' (guarantee) and 'limits' (throttle) for CPU/Memory."
			case strings.Contains(risk, "hpa"), strings.Contains(risk, "autoscaling"):
				return "Action: kubectl autoscale deployment <name> --cpu-percent=80 --min=2 --max=10"
			case strings.Contains(risk, "oom"), strings.Contains(risk, "memory"):
				return "Investigate: Check 'kubectl top pod'. Increase memory limit or debug leak."

			
			case strings.Contains(risk, "latest tag"), strings.Contains(risk, "mutable"):
				return "Fix: Pin image to a specific SHA256 digest or immutable version (e.g., :v1.2.0)."
			case strings.Contains(risk, "exposed"), strings.Contains(risk, "loadbalancer"), strings.Contains(risk, "internet"):
				return "Review: Ensure public exposure is behind a WAF/Ingress Controller, not direct LB."
			case strings.Contains(risk, "prestop"), strings.Contains(risk, "graceful"):
				return "Fix: Add 'lifecycle.preStop' hook to handle connection draining."
			default:
				return "Consult your team's SRE Best Practices documentation."
			}
		}

		for _, factor := range msg.Profile.RiskFactors {
			
			parts := strings.SplitN(factor, ": ", 2)
			category := "Risk"
			reason := factor

			if len(parts) == 2 {
				category = parts[0]
				reason = parts[1]
			}

			mappedResults = append(mappedResults, helm.HeuristicResult{
				Severity:    helm.SevHigh,
				Category:    helm.Category(category),
				Title:       fmt.Sprintf("%s Violation", category),
				Symptom:     reason,
				Description: "Detected by SRE Probabilistic Model",
				Remediation: getRemediation(reason), 
				ScoreImpact: 15.0,
				Calculation: "Penalty Applied",
			})
		}

		report := helm.SREAnalysis{
			Score:             msg.Profile.Score,
			SafetyGrade:       msg.Profile.Grade,
			Results:           mappedResults,
			PrimaryRiskDriver: msg.Profile.CriticalityTier,
		}

		m.srePanel.SetAnalysis(report)
		m.srePanel.Active = true
		m.showSREPanel = true
		m.statusMsg = fmt.Sprintf("SRE Scan: %s", msg.Service)
		return m, nil
	
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.xrayTree.Width = msg.Width
		m.xrayTree.Height = msg.Height
		m.syncTableScroll()

	case rollbackDiffMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Rollback Check Failed: %v", msg.err)
		} else {
			m.rollbackDiffContent = msg.content
			m.rollbackTargetRev = msg.targetRev
			m.showRollbackDiff = true
			headerH := 3
			footerH := 3
			vpHeight := m.height - headerH - footerH
			if vpHeight < 5 {
				vpHeight = 5
			}
			m.rollbackViewport = viewport.New(m.width, vpHeight)
			m.rollbackViewport.SetContent(msg.content)
		}
		return m, nil
	case XRayLoadedMsg:
		m.statusMsg = "X-Ray Scan Complete."
		m.xrayTree.SetRoot(msg.Root)
		m.xrayTree.Width = m.width
		m.xrayTree.Height = m.height
		return m, nil

	
	
	case xRayReadyMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("❌ X-Ray Failed: %v", msg.err)
			m.showXRay = false 
			return m, nil
		}
		
		
		m.xrayTree.SetGraph(msg.graph)
		m.showXRay = true 
		m.statusMsg = "X-Ray Active: [Tab] Layout  [Esc] Close"
		return m, nil

	
	case statsResultMsg:
		if msg.err == nil {
			m.cpuUsage = msg.cpu
			m.memUsage = msg.mem
		} else {
			log.Printf("Metrics Error: %v", msg.err)
			if m.cpuUsage == "" || m.cpuUsage == "..." {
			m.cpuUsage = "Err"
			m.memUsage = "Err"
			}
		}
		
		return m, m.tickStats()
	}

	if m.showHelmDash {
		return m.updateHelmDashboard(msg)
	}

	if m.showXRay {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "esc" || key.String() == "q" {
				m.showXRay = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.xrayTree, cmd = m.xrayTree.Update(msg)
		return m, cmd
	}

	
	if m.confirmMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				if m.confirmInput.Value() == m.confirmTarget {
					m.confirmMode = false
					m.showBlastRadius = false
					m.confirmInput.Blur()
					m.confirmInput.SetValue("")
					m.statusMsg = "Confirmed. Executing action..."
					return m, m.pendingAction()
				} else {
					m.statusMsg = "❌ Name mismatch. Action cancelled."
					m.confirmMode = false
					m.showBlastRadius = false
					m.confirmInput.Blur()
					return m, nil
				}
			case tea.KeyEsc:
				m.confirmMode = false
				m.showBlastRadius = false
				m.confirmInput.Blur()
				m.statusMsg = "Action cancelled by user."
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.confirmInput, cmd = m.confirmInput.Update(msg)
		return m, cmd
	}

	switch msg.(type) {
	
	case statsTickMsg:
		return m, m.fetchClusterStats()

	case metricsTickMsg:
		if m.currentView.Title() == "Pods" {
			m.fetchPodMetrics()
		} else if m.currentView.Title() == "Nodes" {
			m.fetchNodeMetrics()
		}
		m.refreshData()
		return m, m.tickMetrics()
	}
	
	if m.showContexts {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.showContexts = false
				m.statusMsg = "Cancelled context switch"
			case "up", "k":
				if m.contextIdx > 0 {
					m.contextIdx--
				}
			case "down", "j":
				if m.contextIdx < len(m.contextList)-1 {
					m.contextIdx++
				}
			case "enter":
				selectedCtx := m.contextList[m.contextIdx]
				m.statusMsg = fmt.Sprintf("Switching to context: %s...", selectedCtx)
				m.showContexts = false
				return m, func() tea.Msg {
					if m.informerManager != nil {
						m.informerManager.Stop()
					}
					err := m.k8sClient.SwitchContext(selectedCtx)
					if err != nil {
						m.statusMsg = fmt.Sprintf("Switch Failed: %v", err)
					}
					m.clusterInfo = m.k8sClient.GetConfigInfo()
					m.setupInformers()
					m.selectedIdx = 0
					m.tableStartIdx = 0
					m.resources = nil
					m.rawResources = nil
					m.namespace = "default"
					m.refreshData()
					return informer.ResourceChangeMsg{}
				}
			}
		}
		return m, nil
	}
	if m.showContainers {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.showContainers = false
				m.statusMsg = "Log selection cancelled"
			case "up", "k":
				if m.containerIdx > 0 {
					m.containerIdx--
				}
			case "down", "j":
				if m.containerIdx < len(m.containerList)-1 {
					m.containerIdx++
				}
			case "enter":
				selectedContainer := m.containerList[m.containerIdx]
				m.showContainers = false
				return m.viewLogs(selectedContainer)
			}
		}
		return m, nil
	}

	if m.inputMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				m.namespace = m.textInput.Value()
				m.inputMode = false
				m.textInput.Blur()
				m.selectedIdx = 0
				m.tableStartIdx = 0
				displayNs := m.namespace
				if displayNs == "" {
					displayNs = "ALL"
				}
				m.statusMsg = fmt.Sprintf("Switched to namespace: %s", displayNs)
				m.refreshData()
				return m, nil
			case tea.KeyEsc:
				m.inputMode = false
				m.textInput.Blur()
				m.statusMsg = "Cancelled namespace switch"
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
	if m.filterMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter, tea.KeyEsc:
				m.filterMode = false
				m.filterInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		if m.currentView.Title() == "Helm Charts" {
			return m, tea.Batch(cmd, m.debounceRepoSearch(m.filterInput.Value()))
		}
		m.performFilter()
		return m, cmd
	}
	if m.cmdMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				cmdVal := strings.TrimSpace(m.cmdInput.Value())
				if m.pendingInstall {
					m.pendingInstall = false
					m.cmdMode = false
					m.cmdInput.Blur()
					m.cmdInput.SetValue("")
					m.statusMsg = fmt.Sprintf("Installing %s...", cmdVal)
					return m, m.installSelectedChart(cmdVal)
				}
				m.cmdMode = false
				m.cmdInput.Blur()
				m.cmdInput.SetValue("")
				if cmdVal == "ctx" {
					m.contextList = m.k8sClient.ListContexts()
					m.contextIdx = 0
					m.showContexts = true
					m.statusMsg = "Select Context (<Enter> Confirm, <Esc> Cancel)"
					return m, nil
				} else if cmdVal == "q" || cmdVal == "quit" {
					m.quitting = true
					m.informerManager.Stop()
					return m, tea.Quit

				} else if cmdVal == "reload" {
					m.statusMsg = "🔄 Reloading API resources..."
					m.isDiscovering = true
					return m, m.performDiscovery()

				} else {
					found := false
					for key, v := range m.views {
						if strings.EqualFold(key, cmdVal) || strings.EqualFold(v.Title(), cmdVal) {
							m.switchView(key, v.Title())
							found = true
							break
						}
					}
					if !found {
        
        				if aliases, ok := semanticAliases[strings.ToLower(cmdVal)]; ok {
            				for _, alias := range aliases {
               
                				for key, v := range m.views {
                   
                    				if strings.Contains(strings.ToLower(key), strings.ToLower(alias)) || 
                       				   strings.Contains(strings.ToLower(v.Title()), strings.ToLower(alias)) {
										m.switchView(key, v.Title())
										found = true
										m.statusMsg = fmt.Sprintf("Redirected '%s' -> %s", cmdVal, v.Title())
										break
                   				    }
               				   }
                			   if found { break }
           				    }
        				}
    				}
					if !found {
						m.statusMsg = fmt.Sprintf("Unknown command or view: %s", cmdVal)
					}
				}
				return m, nil
			case tea.KeyEsc:
				m.pendingInstall = false
				m.cmdMode = false
				m.cmdInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
	if m.pfMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				ports := m.pfInput.Value()
				m.pfMode = false
				m.pfInput.Blur()
				m.pfInput.SetValue("")

				target := m.activeEditResource

				_, err := m.pfManager.Start(target.Name, target.Namespace, ports)
				if err != nil {
					m.statusMsg = fmt.Sprintf("Forward Failed: %v", err)
				} else {
					m.statusMsg = fmt.Sprintf("🚀 Forwarding %s -> %s", ports, target.Name)
				}
				return m, nil
			case tea.KeyEsc:
				m.pfMode = false
				m.pfInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.pfInput, cmd = m.pfInput.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case informer.ResourceChangeMsg:
		m.refreshData()
		return m, m.waitForK8sUpdates()

	case tea.KeyMsg:
			if !m.inputMode && !m.filterMode && !m.cmdMode && !m.pfMode && len(m.resources) > 0 {
					
					if actionable, ok := m.currentView.(Actionable); ok {
							if handled, cmd := actionable.HandleKey(msg, m.resources[m.selectedIdx]); handled {
									return m, cmd
							}
					}
			}
			
			if !m.inputMode && !m.filterMode && !m.cmdMode && !m.pfMode && len(m.resources) > 0 {
				   
					currentKind := strings.ToLower(m.currentView.Title())

					if plugins, ok := m.plugins[currentKind]; ok {
							for _, p := range plugins {
								if msg.String() == p.ShortCut {
									
									selected := m.resources[m.selectedIdx]
									
									
									fullCmdStr := p.InterpolateCommand(selected)
									
									m.statusMsg = fmt.Sprintf("Running plugin: %s", p.Description)

									
									c := exec.Command("bash", "-c", fullCmdStr)
									return m, tea.ExecProcess(c, func(err error) tea.Msg {
										if err != nil {
											return helmFinishedMsg{err: fmt.Errorf("Plugin failed: %v", err)}
										}
										return helmFinishedMsg{msg: "Plugin executed successfully."}
									})
								}
							}
						}
					}
		
		if _, ok := m.currentView.(*ServiceView); ok {
			switch msg.String() {
			case "b", "E", "H", "r":
			}
		}

		if msg.String() == "S" {
			if m.currentView.Title() == "Helm Charts" {
				m.statusMsg = "Reloading Repo Index..."
				return m, m.loadRepoIndex()
			}
			
			
			m.switchView("charts", "Helm Charts")
			m.statusMsg = "Loading Helm App Store (Parsing Index)..."
			
			
			return m, m.loadRepoIndex() 
		}
		
		if msg.String() == "i" {
			if !m.viewingForwards {
				if m.currentView.Title() == "Helm Charts" {
					return m.promptInstallChart()
				}
				if m.currentView.Title() == "Helm Releases" && len(m.resources) > 0 {
					return m.openDashboard(m.resources[m.selectedIdx], TabOverview)
				}
			}
		}

		if msg.String() == "esc" || msg.String() == "backspace" {
			if strings.HasPrefix(m.currentView.Title(), "Policy:") || m.currentView.Title() == "RBAC Policy" {
				m.switchView("rbac", "RBAC Subjects")
				return m, nil
			}
			if m.currentView.Title() == "Helm Charts" {
				m.switchView("helm", "Helm Releases")
				return m, nil
			}
			if m.diffBaseResource != nil {
				m.diffBaseResource = nil
				m.statusMsg = "Diff selection cancelled."
				return m, nil
			}
		}

		if m.currentView.Title() == "Helm Releases" && !m.viewingForwards {
			if model, cmd := m.handleHelmKeys(msg); model != nil {
				return model, cmd
			}
		}

		if msg.String() == "ctrl+r" {
			if !m.viewingForwards {
				m.populateResourceList()
				m.showResources = true
				m.filterInput.SetValue("")
				m.filterInput.Placeholder = "Search API resources (CRDs)..."
				m.filterInput.Focus()
				return m, textinput.Blink
			}
		}

		switch msg.String() {
		case "tab":
			keys := make([]string, 0, len(m.views))
			for k := range m.views {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			currIdx := -1
			for i, k := range keys {
				if m.views[k] == m.currentView {
					currIdx = i
					break
				}
			}
			nextIdx := (currIdx + 1) % len(keys)
			m.switchView(keys[nextIdx], m.views[keys[nextIdx]].Title())
			return m, nil

		case "shift+tab":
			keys := make([]string, 0, len(m.views))
			for k := range m.views {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			currIdx := -1
			for i, k := range keys {
				if m.views[k] == m.currentView {
					currIdx = i
					break
				}
			}
			nextIdx := (currIdx - 1 + len(keys)) % len(keys)
			m.switchView(keys[nextIdx], m.views[keys[nextIdx]].Title())
			return m, nil

		case "+", "=":
			if !m.viewingForwards && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				if r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "ReplicaSet" {
					m.statusMsg = fmt.Sprintf("Scaling up %s...", r.Name)
					return m, func() tea.Msg {
						_, err := m.k8sClient.ScaleResource(r.Namespace, r.Kind, r.Name, 1)
						if err != nil {
							return informer.ResourceChangeMsg{}
						}
						return informer.ResourceChangeMsg{}
					}
				}
			}
		case "-":
			if !m.viewingForwards && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				if r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "ReplicaSet" {
					m.statusMsg = fmt.Sprintf("Scaling down %s...", r.Name)
					return m, func() tea.Msg {
						_, err := m.k8sClient.ScaleResource(r.Namespace, r.Kind, r.Name, -1)
						if err != nil {
							return informer.ResourceChangeMsg{}
						}
						return informer.ResourceChangeMsg{}
					}
				}
			}
		case "r":
			if !m.viewingForwards && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				if r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "DaemonSet" {
					m.statusMsg = fmt.Sprintf("Triggering restart for %s...", r.Name)
					return m, func() tea.Msg {
						err := m.k8sClient.RestartResource(r.Namespace, r.Kind, r.Name)
						if err != nil {
							m.statusMsg = fmt.Sprintf("Restart Error: %v", err)
						}
						return informer.ResourceChangeMsg{}
					}
				}
			}
		case "h":
			if !m.viewingForwards {
				m.switchView("helm", "Helm Releases")
			}
		case "9":
			m.switchView("events", "Events")
		case "q":
			if m.currentView.Title() == "Events" {
				m.switchView("pods", "Pods")
				return m, nil
			}
			m.quitting = true
			m.informerManager.Stop()
			return m, tea.Quit

		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				if m.currentView.Title() == "Events" {
					m.autoScrollEvents = false
				}
			}
			m.syncTableScroll()
		case "down", "j":
			maxLen := len(m.resources)
			if m.viewingForwards {
				maxLen = len(m.pfManager.List())
			}
			if m.selectedIdx < maxLen-1 {
				m.selectedIdx++
				if m.currentView.Title() == "Events" && m.selectedIdx == maxLen-1 {
					m.autoScrollEvents = true
				}
			}
			m.syncTableScroll()
		case "G":
			if !m.viewingForwards && len(m.resources) > 0 {
				m.selectedIdx = len(m.resources) - 1
				if m.currentView.Title() == "Events" {
					m.autoScrollEvents = true
				}
			}
			m.syncTableScroll()
		case "y":
			if len(m.resources) > 0 && m.selectedIdx < len(m.resources) {
				selected := m.resources[m.selectedIdx]

				
				if selected.Kind == "Chart" || selected.Kind == "Hint" {
					m.statusMsg = "YAML view not available for this item."
					return m, nil
				}

				return m.viewYAML(selected)
			}
			m.syncTableScroll()
		case "enter":
			if !m.viewingForwards {
				if len(m.resources) == 0 {
					return m, nil
				}
				if m.currentView.Title() == "Helm Charts" {
					return m.promptInstallChart()
				}
				if m.currentView.Title() == "RBAC Subjects" {
					selected := m.resources[m.selectedIdx]
					subjects := []auth.SubjectRef{{Name: selected.Name, Kind: selected.Kind, Namespace: selected.Namespace}}
					m.currentView = NewRBACPolicyView(m.informerManager.StaticFactory, subjects, fmt.Sprintf("Policy: %s", selected.Name))
					m.selectedIdx = 0
					m.tableStartIdx = 0
					m.filterInput.SetValue("")
					m.statusMsg = fmt.Sprintf("Viewing permissions for %s", selected.Name)
					m.refreshData()
					return m, nil
				}
				if strings.HasPrefix(m.currentView.Title(), "Policy:") || m.currentView.Title() == "RBAC Policy" {
					return m, nil
				}
				if m.currentView.Title() == "Nodes" {
					return m.viewDescribe()
				}
				if m.currentView.Title() == "Events" {
					return m.viewEventDetail()
				}
				return m.viewYAML(m.resources[m.selectedIdx])
			}
		case ":":
			if !m.viewingForwards {
				m.cmdMode = true
				m.cmdInput.Focus()
				return m, textinput.Blink
			}
		case "n":
			if !m.viewingForwards {
				m.showNamespaces = true
				m.statusMsg = "Fetching namespaces..."
				m.fetchNamespaces()
				return m, textinput.Blink
			}
		case "/":
			if !m.viewingForwards {
				m.filterMode = true
				m.filterInput.Focus()
				m.filterInput.SetValue("")
				if m.currentView.Title() == "Helm Charts" {
					m.performAsyncRepoSearch("")
				} else {
					m.performFilter()
				}
				return m, textinput.Blink
			}
		case "c":
			if len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				val := r.Name
				if m.currentView.Title() == "Events" {
					val = fmt.Sprintf("[%s] %s/%s\nType: %s\nReason: %s\nMessage: %s", r.AgeRaw.Format(time.RFC3339), r.Namespace, r.Name, r.Extras[0], r.Extras[1], r.Extras[3])
				}
				m.copyToClipboard(val)
				return m, nil
			}
		case "D":
			if !m.viewingForwards {
				if len(m.resources) > 0 {
					current := m.resources[m.selectedIdx]
					if m.diffBaseResource == nil {
						baseCopy := current
						m.diffBaseResource = &baseCopy
						m.statusMsg = fmt.Sprintf("📍 Diff Source: %s. Select target and press 'D' again.", current.Name)
					} else {
						base := *m.diffBaseResource
						target := current
						m.diffBaseResource = nil
						if base.Name == target.Name && base.Namespace == target.Namespace {
							m.statusMsg = "Cancelled: Cannot diff identical resources."
							return m, nil
						}
						m.statusMsg = fmt.Sprintf("Diffing %s vs %s...", base.Name, target.Name)
						return m, func() tea.Msg {
							yamlA, errA := m.fetchInternalYAML(base)
							if errA != nil {
								return textWindowMsg{title: "Error", content: fmt.Sprintf("Error fetching Base: %v", errA), status: "Error"}
							}
							yamlB, errB := m.fetchInternalYAML(target)
							if errB != nil {
								return textWindowMsg{title: "Error", content: fmt.Sprintf("Error fetching Target: %v", errB), status: "Error"}
							}
							diffOut := utils.ComputeDiff(base.Name, yamlA, target.Name, yamlB, m.width)
							return textWindowMsg{title: "Resource Diff (Side-by-Side)", content: diffOut, status: "Diff Calculated"}
						}
					}
				}
			}
		case "f":
			if !m.viewingForwards && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				targetPod, err := m.resolvePodForForwarding(r)
				if err != nil {
					m.statusMsg = fmt.Sprintf("PF Error: %v", err)
					return m, nil
				}
				m.activeEditResource = targetPod
				m.pfMode = true
				m.pfInput.Focus()
				m.statusMsg = fmt.Sprintf("Forwarding %s/%s (Resolved from %s)", targetPod.Namespace, targetPod.Name, r.Kind)
				return m, textinput.Blink
			}
		case "F":
			m.viewingForwards = !m.viewingForwards
			m.selectedIdx = 0
			m.tableStartIdx = 0
			if m.viewingForwards {
				m.statusMsg = "Viewing Active Port Forwards"
			} else {
				m.statusMsg = "Returned to Explorer"
			}
		case "N":
			if !m.viewingForwards {
				m.setSort("name")
				m.statusMsg = fmt.Sprintf("Sorting by Name (%v)", m.sortAsc)
			}
		case "A":
			if !m.viewingForwards {
				m.setSort("age")
				m.statusMsg = fmt.Sprintf("Sorting by Age (%v)", m.sortAsc)
			}
		case "R":
			if !m.viewingForwards {
				m.setSort("restarts")
				m.statusMsg = fmt.Sprintf("Sorting by Restarts (%v)", m.sortAsc)
			}
		case "M":
			if !m.viewingForwards {
				m.setSort("mem")
				m.statusMsg = fmt.Sprintf("Sorting by Memory (%v)", m.sortAsc)
			}
		case "P", "p":
			if !m.viewingForwards {
				m.setSort("cpu")
				m.statusMsg = fmt.Sprintf("Sorting by CPU (%v)", m.sortAsc)
			}
		case "d":
			if m.currentView.Title() == "Nodes" && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				m.statusMsg = fmt.Sprintf("Draining node %s... (Check terminal if blocked)", r.Name)
				return m, func() tea.Msg {
					cmd := exec.Command("kubectl", "drain", r.Name, "--ignore-daemonsets", "--delete-emptydir-data", "--force")
					return tea.ExecProcess(cmd, func(err error) tea.Msg {
						if err != nil {
						}
						return informer.ResourceChangeMsg{}
					})
				}
			}
			if !m.viewingForwards {
				return m.viewDescribe()
			}
		case "C":
			if m.currentView.Title() == "Nodes" && len(m.resources) > 0 {
				r := m.resources[m.selectedIdx]
				m.statusMsg = fmt.Sprintf("Toggling Cordon for %s...", r.Name)
				return m.toggleCordon(r.Name)
			}
		case "l":
			if !m.viewingForwards {
				return m.prepareLogs()
			}
		case "s":
			if !m.viewingForwards {
				return m.viewShell()
			}

		case "ctrl+d":
            if !m.viewingForwards {
                return m.debugPod()
            }
			
		case "e":
			if !m.viewingForwards {
				if m.currentView.Title() == "Helm Releases" {
					return m.editHelmValues()
				}
				return m.editSelectedResource()
			}
		case "x":
			if m.viewingForwards {
				list := m.pfManager.List()
				if len(list) > 0 && m.selectedIdx < len(list) {
					target := list[m.selectedIdx]
					m.pfManager.Stop(target.ID)
					m.statusMsg = fmt.Sprintf("Stopped Forward: %s", target.ID)
					if m.selectedIdx >= len(list)-1 && m.selectedIdx > 0 {
						m.selectedIdx--
					}
				}
			} else {
				m.deleteSelectedResource()
			}
		case "z":
		
			if !m.viewingForwards && len(m.resources) > 0 {
				return m.triggerXRay()
			}
		case "1":
			m.switchView("pods", "Pods")
		case "2":
			m.switchView("deployments", "Deployments")
		case "3":
			m.switchView("services", "Services")
		case "4":
			m.switchView("replicasets", "ReplicaSets")
		case "5":
			m.switchView("configmaps", "ConfigMaps")
		case "6":
			m.switchView("secrets", "Secrets")
		case "7":
			m.switchView("nodes", "Nodes")
		case "J":
			m.switchView("jobs", "Jobs")
		case "O":
			m.switchView("cronjobs", "CronJobs")
		case "8":
			user := m.clusterInfo.User
			subjects := []auth.SubjectRef{{Kind: "User", Name: user}, {Kind: "Group", Name: "system:authenticated"}, {Kind: "Group", Name: "system:masters"}}
			m.currentView = NewRBACPolicyView(m.informerManager.StaticFactory, subjects, fmt.Sprintf("Policy: %s", user))
			m.selectedIdx = 0
			m.tableStartIdx = 0
			m.filterInput.SetValue("")
			m.statusMsg = fmt.Sprintf("RBAC Matrix for %s (Press <Esc> for All Subjects)", user)
			m.refreshData()
		}
	}

	return m, nil
}

func (m *ExplorerModel) toggleCordon(nodeName string) (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		ctx := context.TODO()
		client := m.k8sClient.GetClientset()
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return helmFinishedMsg{err: fmt.Errorf("cordon get failed: %v", err)}
		}
		desired := !node.Spec.Unschedulable
		payload := fmt.Sprintf(`{"spec":{"unschedulable":%v}}`, desired)
		_, err = client.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, []byte(payload), metav1.PatchOptions{})
		if err != nil {
			return helmFinishedMsg{err: fmt.Errorf("cordon patch failed: %v", err)}
		}
		action := "Cordoned"
		if !desired {
			action = "Uncordoned"
		}
		return helmFinishedMsg{msg: fmt.Sprintf("%s node %s", action, nodeName)}
	}
}

func (m *ExplorerModel) switchView(key, name string) {
	if view, exists := m.views[key]; exists {
		m.currentView = view
		m.selectedIdx = 0
		m.tableStartIdx = 0
		m.viewingForwards = false
		m.statusMsg = ""
		m.filterInput.SetValue("")
		if key == "events" {
			m.sortBy = "age"
			m.sortAsc = true
			m.autoScrollEvents = true
		} else {
			m.sortBy = "name"
			m.sortAsc = true
		}
		m.refreshData()
	}
}

func (m *ExplorerModel) refreshData() {
	arg := m.namespace
	if m.currentView.Title() == "Helm Charts" {
		if len(m.resources) == 0 {
			m.performAsyncRepoSearch("")
		}
		return
	}

	raw, err := m.currentView.Retrieve(arg)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", err)
		return
	}
	m.rawResources = raw

	m.performFilter()

	if m.currentView.Title() == "Events" && m.autoScrollEvents {
		if len(m.resources) > 0 {
			m.selectedIdx = len(m.resources) - 1
		}
	}
	m.clampCursor()
	m.syncTableScroll()
}

func (m *ExplorerModel) editSelectedResource() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]
	m.activeEditResource = r

	cmd := exec.Command("kubectl", "get", r.Kind, r.Name, "-n", r.Namespace, "-o", "yaml")
	if r.Kind == "Node" || m.currentView.Title() == "Nodes" {
		cmd = exec.Command("kubectl", "get", "node", r.Name, "-o", "yaml")
	} else if r.Namespace == "" {
		cmd = exec.Command("kubectl", "get", r.Kind, r.Name, "-o", "yaml")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to fetch YAML: %v", err)
		return m, nil
	}
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kubevision-edit-%s-*.yaml", r.Name))
	if err != nil {
		return m, nil
	}
	if _, err := tmpFile.Write(out); err != nil {
		return m, nil
	}
	tmpFile.Close()
	m.activeEditFile = tmpFile.Name()
	c := utils.BuildEditorCmd(m.activeEditFile)
	return m, tea.ExecProcess(c, func(err error) tea.Msg { return editorFinishedMsg{err: err} })
}


func (m *ExplorerModel) deleteSelectedResource() {
	if len(m.resources) == 0 {
		return
	}
	r := m.resources[m.selectedIdx]

	
	gvr := m.currentView.GetGVR()
	
	if gvr.Resource == "" {
		m.statusMsg = "Deletion not supported for this view."
		return
	}


	m.confirmMode = true
	m.confirmTarget = r.Name
	m.confirmInput.SetValue("")
	m.confirmInput.Focus()


	m.pendingAction = func() tea.Cmd {
		return m.finalizeDeletion(r, gvr)
	}

	m.statusMsg = fmt.Sprintf("⚠️  DELETE: Type '%s' to confirm destruction.", r.Name)
}


func (m *ExplorerModel) finalizeDeletion(r Resource, gvr schema.GroupVersionResource) tea.Cmd {
	return func() tea.Msg {
		
		err := m.k8sClient.GetDynamicClient().Resource(gvr).Namespace(r.Namespace).Delete(context.TODO(), r.Name, metav1.DeleteOptions{})

		
		if err != nil {
			cmd := exec.Command("kubectl", "delete", gvr.Resource, r.Name, "-n", r.Namespace)
			if out, execErr := cmd.CombinedOutput(); execErr != nil {
				
				return helmFinishedMsg{err: fmt.Errorf("Delete Failed: %v | %s", err, string(out))}
			}
		}

		return helmFinishedMsg{msg: fmt.Sprintf("🗑️  Deleted %s %s/%s", gvr.Resource, r.Namespace, r.Name)}
	}
}

func (m *ExplorerModel) viewDescribe() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]
	var cmd *exec.Cmd

	resourceType := r.Kind
	
	if v, ok := m.currentView.(interface{ GetGVR() schema.GroupVersionResource }); ok {
		resourceType = v.GetGVR().Resource
	}

	if r.Kind == "Node" || m.currentView.Title() == "Nodes" {
		cmd = exec.Command("kubectl", "describe", "node", r.Name)
	} else {
		cmd = exec.Command("kubectl", "describe", resourceType, r.Name, "-n", r.Namespace)
	}
	out, err := cmd.CombinedOutput()
	content := string(out)
	if err != nil {
		content = fmt.Sprintf("Error running kubectl describe:\n%v\n\nOutput:\n%s", err, string(out))
	}
	m.textViewer = text.NewTextViewer(fmt.Sprintf("Describe: %s/%s", r.Kind, r.Name), content, m.width, m.height)
	m.showText = true
	return m, nil
}

func (m *ExplorerModel) viewEventDetail() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("EVENT DETAIL: %s\n", r.Name))
	b.WriteString(strings.Repeat("-", 50) + "\n")
	b.WriteString(fmt.Sprintf("Namespace: %s\n", r.Namespace))
	b.WriteString(fmt.Sprintf("Time:      %s\n", r.AgeRaw.Format(time.RFC1123)))
	b.WriteString(fmt.Sprintf("Type:      %s\n", r.Extras[0]))
	b.WriteString(fmt.Sprintf("Reason:    %s\n", r.Extras[1]))
	b.WriteString(fmt.Sprintf("Object:    %s\n", r.Extras[2]))
	b.WriteString(strings.Repeat("-", 50) + "\n\n")
	b.WriteString("MESSAGE:\n")
	b.WriteString(r.Extras[3])
	m.textViewer = text.NewTextViewer(fmt.Sprintf("Event: %s", r.Name), b.String(), m.width, m.height)
	m.showText = true
	return m, nil
}

func (m *ExplorerModel) prepareLogs() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]

	
	if r.Kind == "Pod" || m.currentView.Title() == "Pods" {
		if r.Kind != "Pod" && m.currentView.Title() != "Pods" {
			m.statusMsg = "Logs only available for Pods"
			return m, nil
		}

		ctx := context.TODO()
		pod, err := m.k8sClient.GetClientset().CoreV1().Pods(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to get pod details: %v", err)
			return m, nil
		}
		var containers []string
		for _, c := range pod.Spec.Containers {
			containers = append(containers, c.Name)
		}
		for _, c := range pod.Spec.InitContainers {
			containers = append(containers, c.Name+" (init)")
		}
		if len(containers) > 1 {
			m.targetLogPod = r
			m.containerList = containers
			m.containerIdx = 0
			m.showContainers = true
			m.statusMsg = "Select Container for Logs"
			return m, nil
		}
		m.targetLogPod = r
		return m.viewLogs("")
	}

	
	supportedKinds := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true,
	}

	if supportedKinds[r.Kind] {
		m.statusMsg = fmt.Sprintf("Aggregating logs for %s/%s...", r.Kind, r.Name)

		return m, func() tea.Msg {
			client := m.k8sClient.GetClientset()
			ctx := context.TODO()
			var selectorStr string
			var err error

			switch r.Kind {
			case "Deployment":
				obj, err := client.AppsV1().Deployments(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
				if err == nil {
					selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
				}
			case "StatefulSet":
				obj, err := client.AppsV1().StatefulSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
				if err == nil {
					selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
				}
			case "DaemonSet":
				obj, err := client.AppsV1().DaemonSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
				if err == nil {
					selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
				}
			case "ReplicaSet":
				obj, err := client.AppsV1().ReplicaSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
				if err == nil {
					selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
				}
			}

			if err != nil {
				return helmFinishedMsg{err: fmt.Errorf("failed to get resource: %v", err)}
			}
			if selectorStr == "" {
				return helmFinishedMsg{err: fmt.Errorf("could not resolve selector for %s", r.Name)}
			}

			
			podList, err := client.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selectorStr})
			if err != nil {
				return helmFinishedMsg{err: fmt.Errorf("failed to list pods: %v", err)}
			}

			if len(podList.Items) == 0 {
				return helmFinishedMsg{msg: "No active pods found for this resource."}
			}

			
			stream, err := logs.StreamMultiPodLogs(context.Background(), client, podList.Items)
			if err != nil {
				return helmFinishedMsg{err: err}
			}

			return streamReadyMsg{
				stream:    stream,
				title:     fmt.Sprintf("Logs: %s [Multi-Pod: %d]", r.Name, len(podList.Items)),
				timeLabel: "All",
				podName:   r.Name, 
			}
		}
	}

	m.statusMsg = "Logs not supported for this resource type"
	return m, nil
}


func viewLogs(m *ExplorerModel, containerName string) (tea.Model, tea.Cmd) {
	return m.viewLogs(containerName)
}

func (m *ExplorerModel) viewLogs(containerName string) (tea.Model, tea.Cmd) {
	r := m.targetLogPod
	realContainerName := strings.Split(containerName, " ")[0]
	opts := &corev1.PodLogOptions{
		Follow:     true,
		TailLines:  int64Ptr(2000),
		Timestamps: true,
	}
	if realContainerName != "" {
		opts.Container = realContainerName
	}
	req := m.k8sClient.GetClientset().CoreV1().Pods(r.Namespace).GetLogs(r.Name, opts)
	stream, err := req.Stream(context.TODO())
	if err != nil {
		m.statusMsg = fmt.Sprintf("Log error: %v", err)
		return m, nil
	}
	title := fmt.Sprintf("Logs: %s", r.Name)
	if realContainerName != "" {
		title += fmt.Sprintf(" [%s]", realContainerName)
	}
	m.logViewer = logs.NewLogViewer(title, r.Name, m.width, m.height, stream)
	m.showLogs = true
	return m, m.logViewer.Init()
}

func (m *ExplorerModel) viewShell() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	r := m.resources[m.selectedIdx]
	if r.Kind != "Pod" && m.currentView.Title() != "Pods" {
		m.statusMsg = "Shell only available for Pods"
		return m, nil
	}
	baseCmd := fmt.Sprintf("kubectl exec -it %s -n %s -- sh", r.Name, r.Namespace)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		fullCmd := fmt.Sprintf("%s || echo 'Command failed'; echo; read -p 'Press Enter to close...' var", baseCmd)
		if utils.IsCommandAvailable("gnome-terminal") {
			cmd = exec.Command("gnome-terminal", "--", "bash", "-c", fullCmd)
		} else if utils.IsCommandAvailable("xterm") {
			cmd = exec.Command("xterm", "-e", "bash", "-c", fullCmd)
		} else {
			m.statusMsg = "No supported terminal found"
			return m, nil
		}
	case "darwin":
		escapedCmd := strings.ReplaceAll(baseCmd, "\"", "\\\"")
		script := fmt.Sprintf(`tell application "Terminal" to do script "%s"`, escapedCmd)
		cmd = exec.Command("osascript", "-e", script)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "Kubevision Shell", "cmd", "/k", baseCmd)
	default:
		m.statusMsg = "Unsupported OS for external shell"
		return m, nil
	}
	err := cmd.Start()
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to launch shell: %v", err)
	}
	return m, nil
}


func (m *ExplorerModel) debugPod() (tea.Model, tea.Cmd) {
    if len(m.resources) == 0 {
        return m, nil
    }
    r := m.resources[m.selectedIdx]

    
    targetPod, err := m.resolvePodForForwarding(r)
    if err != nil {
        
        if r.Kind == "Pod" {
            targetPod = r
        } else {
            m.statusMsg = fmt.Sprintf("Debug Error: %v", err)
            return m, nil
        }
    }

    m.statusMsg = fmt.Sprintf("🪲 Injecting Netshoot into %s/%s...", targetPod.Namespace, targetPod.Name)

 
    baseCmd := fmt.Sprintf("kubectl debug -it %s -n %s --image=nicolaka/netshoot --share-processes --profile=sysadmin -- sh", targetPod.Name, targetPod.Namespace)

    
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "linux":
        fullCmd := fmt.Sprintf("%s || echo 'Command failed'; echo; read -p 'Press Enter to close...' var", baseCmd)
        if utils.IsCommandAvailable("gnome-terminal") {
            cmd = exec.Command("gnome-terminal", "--", "bash", "-c", fullCmd)
        } else if utils.IsCommandAvailable("xterm") {
            cmd = exec.Command("xterm", "-e", "bash", "-c", fullCmd)
        } else {
            m.statusMsg = "No supported terminal found (install gnome-terminal or xterm)"
            return m, nil
        }
    case "darwin":
        escapedCmd := strings.ReplaceAll(baseCmd, "\"", "\\\"")
        script := fmt.Sprintf(`tell application "Terminal" to do script "%s"`, escapedCmd)
        cmd = exec.Command("osascript", "-e", script)
    case "windows":
        cmd = exec.Command("cmd", "/c", "start", "Kubevision SRE Debug", "cmd", "/k", baseCmd)
    default:
        m.statusMsg = "Unsupported OS for external debug shell"
        return m, nil
    }

    err = cmd.Start()
    if err != nil {
        m.statusMsg = fmt.Sprintf("Failed to launch debugger: %v", err)
    }
    return m, nil
}



func (m *ExplorerModel) renderHeader() string {
	
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true) 
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))            
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))             
	
	
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF00FF")).
		Bold(true).
		Width(6).
		Align(lipgloss.Center)

	
	col1 := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Left, labelStyle.Render("Context: "), valueStyle.Render(m.clusterInfo.Context)),
		lipgloss.JoinHorizontal(lipgloss.Left, labelStyle.Render("Cluster: "), valueStyle.Render(m.clusterInfo.Cluster)),
		lipgloss.JoinHorizontal(lipgloss.Left, labelStyle.Render("User:    "), valueStyle.Render(m.clusterInfo.User)),
		lipgloss.JoinHorizontal(lipgloss.Left, labelStyle.Render("CPU:     "), valueStyle.Render(m.cpuUsage)),
		lipgloss.JoinHorizontal(lipgloss.Left, labelStyle.Render("Mem:     "), valueStyle.Render(m.memUsage)),
	)

	
	makeBlock := func(key, desc string) string {
		k := keyStyle.Render("<" + key + ">")
		d := descStyle.Render(desc)
		
		return lipgloss.NewStyle().Width(20).Render(lipgloss.JoinHorizontal(lipgloss.Left, k, " ", d))
	}

	
	row1 := lipgloss.JoinHorizontal(lipgloss.Left, 
		makeBlock("1", "Pods"), 
		makeBlock("2", "Deploy"), 
		makeBlock("3", "Service"), 
		makeBlock("n", "Namespace"), 
		makeBlock("^r", "SRE Discovery"), 
	)
	
	
	row2 := lipgloss.JoinHorizontal(lipgloss.Left, 
		makeBlock("^d", "SRE Debug"),     
		makeBlock("7", "Nodes"), 
		makeBlock("8", "RBAC"), 
		makeBlock("9", "Events"), 
		makeBlock("h", "Helm"),
	)
	
	
	row3 := lipgloss.JoinHorizontal(lipgloss.Left, 
		makeBlock(":", "Cmd"), 
		makeBlock("/", "Filter"), 
		makeBlock("l", "Logs"), 
		makeBlock("e", "Edit"), 
		makeBlock("y", "YAML"),
	)
	
	
	row4 := lipgloss.JoinHorizontal(lipgloss.Left, 
		makeBlock("d", "Describe"), 
		makeBlock("D", "Diff"), 
		makeBlock("f", "PortFwd"), 
		makeBlock("F", "FwdMgr"), 
		makeBlock("s", "Shell"),
	)
	
	
	row5 := lipgloss.JoinHorizontal(lipgloss.Left, 
		makeBlock("N", "Sort Name"), 
		makeBlock("P", "Sort CPU"), 
		makeBlock("M", "Sort Mem"), 
		makeBlock("R", "Sort Rst"), 
		makeBlock("A", "Sort Age"),
	)

	
	row6 := ""
	if m.currentView.Title() == "Deployments" {
		row6 = lipgloss.JoinHorizontal(lipgloss.Left, 
			makeBlock("+", "Scale Up"), 
			makeBlock("-", "Scale Dn"), 
			makeBlock("r", "Restart"),
		)
	} else if m.currentView.Title() == "Nodes" {
		row6 = lipgloss.JoinHorizontal(lipgloss.Left, 
			makeBlock("C", "Cordon"), 
			makeBlock("d", "Drain"),
		)
	} else if m.viewingForwards {
		row6 = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render("       [ACTIVE PORT FORWARD MODE]")
	}

	col2 := lipgloss.JoinVertical(lipgloss.Center, row1, row2, row3, row4, row5, row6)

	
	logo := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(`
  _  ___   _ ___ ___ 
 | |/ / | | | _ ) __|
 | ' <| |_| | _ \ _| 
 |_|\_\\___/|___/___|
     VISION v0.1`)

	
	width := m.width
	if width == 0 { width = 100 }


	w1 := int(float64(width) * 0.15)
	w2 := int(float64(width) * 0.70)
	w3 := width - w1 - w2 - 2

	c1 := lipgloss.NewStyle().Width(w1).Render(col1)
	c2 := lipgloss.NewStyle().Width(w2).Align(lipgloss.Center).Render(col2)
	c3 := lipgloss.NewStyle().Width(w3).Align(lipgloss.Right).Render(logo)

	return lipgloss.JoinHorizontal(lipgloss.Top, c1, c2, c3)
}

func (m *ExplorerModel) View() string {
	if m.quitting {
		return "Bye!\n"
	}

	if m.confirmMode && m.showBlastRadius {
		var b strings.Builder
		headerColor := "#FFA500" 
		if m.blastReport.Score >= 50 {
			headerColor = "#FF0000" 
		}

		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color(headerColor)).
			Width(60).
			Align(lipgloss.Center).
			Render(fmt.Sprintf(" 💥 IMPACT ANALYSIS: UNINSTALLING '%s' ", m.confirmTarget)) + "\n\n")

		if len(m.blastReport.Criticals) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("!! CRITICAL IMPACT (Irreversible Data/Net Loss): !!") + "\n")
			for _, c := range m.blastReport.Criticals {
				line := fmt.Sprintf("  • %s %s/%s: %s", styles.GetIcon(c.Kind), c.Kind, c.Name, c.Info)
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}

		if len(m.blastReport.Warnings) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("!! WARNINGS (Configuration Loss): !!") + "\n")
			for _, w := range m.blastReport.Warnings {
				line := fmt.Sprintf("  • %s %s/%s", styles.GetIcon(w.Kind), w.Kind, w.Name)
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}

		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(fmt.Sprintf("  Plus %d standard resources (Pods, Deployments, etc) will be removed.", len(m.blastReport.Standard))) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).Render(fmt.Sprintf("To confirm destruction, type '%s' below:", m.confirmTarget)) + "\n\n")
		b.WriteString(m.confirmInput.View() + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("[Enter] Destroy  [Esc] Cancel"))

		popup := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color(headerColor)).Padding(1, 2).Render(b.String())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.confirmMode {
		var b strings.Builder

	
		baseStyle := lipgloss.NewStyle().Width(50).Align(lipgloss.Center)
		alertStyle := baseStyle.Copy().Bold(true).Foreground(lipgloss.Color("#FF0000"))
		textStyle := baseStyle.Copy().Foreground(lipgloss.Color("#CCCCCC"))
		subtleStyle := baseStyle.Copy().Foreground(lipgloss.Color("#666666"))

		
		b.WriteString(alertStyle.Render("!! DANGER: DESTRUCTIVE ACTION DETECTED !!") + "\n\n")

	
		b.WriteString(baseStyle.Render(fmt.Sprintf("You are about to modify/delete: %s", m.confirmTarget)) + "\n\n")

		
		b.WriteString(textStyle.Render(fmt.Sprintf("To confirm, please type '%s' below:", m.confirmTarget)) + "\n\n")


		b.WriteString(lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(m.confirmInput.View()) + "\n\n")

		
		b.WriteString(subtleStyle.Render("[Enter] Confirm  [Esc] Cancel"))

		
		popup := lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#FF0000")).
			Padding(1, 2).
			Render(b.String())

		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.showContexts {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#005F87")).Foreground(lipgloss.Color("#FFFFFF")).Width(40).Align(lipgloss.Center).Render(" SELECT CONTEXT ") + "\n")
		start := 0
		if m.contextIdx > 10 {
			start = m.contextIdx - 10
		}
		end := start + 20
		if end > len(m.contextList) {
			end = len(m.contextList)
		}
		for i := start; i < end; i++ {
			ctx := m.contextList[i]
			if i == m.contextIdx {
				b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Width(40).Render("> "+ctx) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Width(40).Render("  "+ctx) + "\n")
			}
		}
		popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Render(b.String())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	
	if m.showNamespaces {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#005F87")).Foreground(lipgloss.Color("#FFFFFF")).Width(50).Align(lipgloss.Center).Render(" SWITCH NAMESPACE ") + "\n\n")

		
		b.WriteString(lipgloss.NewStyle().Width(50).Background(lipgloss.Color("#222222")).Render(m.filterInput.View()) + "\n")
		b.WriteString(strings.Repeat("─", 50) + "\n")

		start := 0
		if m.namespaceIdx > 10 {
			start = m.namespaceIdx - 10
		}
		end := start + 15
		if end > len(m.filteredNamespaces) {
			end = len(m.filteredNamespaces)
		}

		if len(m.filteredNamespaces) == 0 {
			b.WriteString("  (No matches found)\n")
		} else {
			for i := start; i < end; i++ {
				ns := m.filteredNamespaces[i]
				if i == m.namespaceIdx {
					b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Width(50).Render("> "+ns) + "\n")
				} else {
					b.WriteString(lipgloss.NewStyle().Width(50).Render("  "+ns) + "\n")
				}
			}
		}

		popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Render(b.String())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.showContainers {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#553388")).Foreground(lipgloss.Color("#FFFFFF")).Width(40).Align(lipgloss.Center).Render(" SELECT CONTAINER ") + "\n")
		for i, c := range m.containerList {
			if i == m.containerIdx {
				b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Width(40).Render("> "+c) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Width(40).Render("  "+c) + "\n")
			}
		}
		popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Render(b.String())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.showRollbackDiff {
		header := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF0000")).
			Bold(true).
			Width(m.width).
			Align(lipgloss.Center).
			Render(fmt.Sprintf(" ⚠️  CONFIRM ROLLBACK TO REVISION %d ", m.rollbackTargetRev))

		content := m.rollbackViewport.View()

		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#333333")).
			Width(m.width).
			Align(lipgloss.Center).
			Render(" [y] Confirm Rollback  |  [n/Esc] Cancel  |  [↑/↓] Scroll ")

		return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
	}

	if m.showUpgradeDiff {
		var header, riskPanel, footer string

		if len(m.upgradeRisks.Factors) > 0 {
			var riskRows []string
			titleColor := "#00FF00"
			titleText := "✅ LOW RISK UPGRADE"
			if m.upgradeRisks.Score >= 50 {
				titleColor = "#FF0000"
				titleText = "⛔ HIGH RISK UPGRADE - REVIEW CAREFULLY"
			} else if m.upgradeRisks.Score >= 20 {
				titleColor = "#FFA500"
				titleText = "⚠️ MEDIUM RISK UPGRADE"
			}

			riskRows = append(riskRows, lipgloss.NewStyle().Foreground(lipgloss.Color(titleColor)).Bold(true).Render(titleText))
			riskRows = append(riskRows, "")

			for _, r := range m.upgradeRisks.Factors {
				color := "#FFFFFF"
				if r.Level == helm.RiskHigh {
					color = "#FF0000"
				}
				if r.Level == helm.RiskMedium {
					color = "#FFA500"
				}

				row := fmt.Sprintf("%s [%s] %s: %s", r.Icon, r.Level, r.Title, r.Message)
				riskRows = append(riskRows, lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(row))
			}

			riskPanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(titleColor)).
				Padding(0, 1).
				Width(m.width - 2).
				Render(strings.Join(riskRows, "\n"))

			riskPanel += "\n"
		}

		if m.upgradeDryRunError {
			header = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#FF0000")).
				Bold(true).
				Width(m.width).
				Align(lipgloss.Center).
				Render(" ⚠️  DRY RUN FAILED - MANUAL OVERRIDE  ⚠️ ")

			footer = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#333333")).
				Width(m.width).
				Align(lipgloss.Center).
				Render(" [y] Force Apply (Blind Upgrade)  |  [n/Esc] Cancel ")
		} else {
			header = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#005F87")).
				Bold(true).
				Width(m.width).
				Align(lipgloss.Center).
				Render(" 🚀 CONFIRM HELM UPGRADE (DRY RUN) ")

			footer = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#333333")).
				Width(m.width).
				Align(lipgloss.Center).
				Render(" [y] Apply Upgrade  |  [n/Esc] Cancel  |  [↑/↓] Scroll ")
		}

		riskHeight := lipgloss.Height(riskPanel)
		vpHeight := m.height - riskHeight - 6
		if vpHeight < 5 {
			vpHeight = 5
		}

		m.rollbackViewport.Height = vpHeight
		content := m.rollbackViewport.View()

		return lipgloss.JoinVertical(lipgloss.Left, header, riskPanel, content, footer)
	}

	if m.showXRay {
		title := fmt.Sprintf(" \U0001f50d X-RAY: %s/%s (Press <ESC> to close) ", m.activeEditResource.Namespace, m.activeEditResource.Name)
		header := lipgloss.NewStyle().Background(lipgloss.Color("#553388")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Width(m.width).Padding(0, 1).Render(title)
		return lipgloss.JoinVertical(lipgloss.Left, header, m.xrayTree.View())
	}
	if m.showLogs && m.logViewer != nil {
		m.logViewer.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m.logViewer.View()
	}
	if m.showYAML && m.yamlViewer != nil {
		m.yamlViewer.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m.yamlViewer.View()
	}
	if m.showText && m.textViewer != nil {
		m.textViewer.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m.textViewer.View()
	}

	if m.showHelmDash && m.helmDashboard != nil {
		dashboardView := m.helmDashboard.View()
		footer := m.renderHelmFooter()
		return lipgloss.JoinVertical(lipgloss.Left, dashboardView, footer)
	}


	if m.showSREPanel {
		return m.srePanel.View()
	}

	if m.showResources {
		return m.viewResourcePicker()
	}

	header := m.renderHeader()

	var title string
	if m.viewingForwards {
		title = fmt.Sprintf(" ACTIVE PORT FORWARDS [Total: %d]", len(m.pfManager.List()))
	} else {
		nsDisplay := m.namespace
		if nsDisplay == "" {
			nsDisplay = "ALL NAMESPACES"
		}
		title = fmt.Sprintf(" %s (%s) [Total: %d]", m.currentView.Title(), nsDisplay, len(m.resources))
		if m.currentView.Title() == "Events" {
			if m.autoScrollEvents {
				title += " [LIVE]"
			} else {
				title += " [PAUSED]"
			}
		}
	}
	titleBar := lipgloss.NewStyle().Background(lipgloss.Color("#005F87")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1).Width(m.width).Render(title)

	var tableContent strings.Builder
	if m.viewingForwards {
		m.renderForwardsTable(&tableContent)
	} else {
		m.renderResourcesTable(&tableContent)
	}

	var statusBar string
	if m.isOffline {
        
        msg := m.statusMsg
        if msg == "" {
            msg = "⚠️  LOST CONNECTION (Retrying...)"
        }

        statusBar = lipgloss.NewStyle().
            Foreground(lipgloss.Color("#FFFFFF")).
            Background(lipgloss.Color("#FF0000")). 
            Bold(true).
            Padding(0, 1).
            Width(m.width).
            Render(msg)

    } else if m.inputMode {
		statusBar = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Width(m.width).Padding(0, 1).Render(m.textInput.View())
	} else if m.pfMode {
		statusBar = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Width(m.width).Padding(0, 1).Render(m.pfInput.View())
	} else if m.filterMode {
		statusBar = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Width(m.width).Padding(0, 1).Render(m.filterInput.View())
	} else if m.cmdMode {
		statusBar = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Width(m.width).Padding(0, 1).Render(m.cmdInput.View())
	} else {
		
		helpText := ""
		if m.currentView.Title() == "Helm Charts" {
			helpText = " <Enter/i> Install Chart | </> Search | <S> Reload Index | <Esc> Back"
		} else if m.currentView.Title() == "Helm Releases" {
			helpText = m.renderHelmFooterString() + " <S> Store"
		} else if m.currentView.Title() == "Events" {
			helpText = " <c> Copy | <Enter> Detail | <G> Bottom (Live) | </> Filter | <q> Back"
		} else if m.currentView.Title() == "Nodes" {
			helpText = " <Enter> Describe | <C> Cordon <d> Drain | <n> Namespace </> Filter <e> Edit YAML"
		} else if m.currentView.Title() == "RBAC Subjects" {
			helpText = " <Enter> View Matrix | <n> Namespace </> Filter"
		} else if strings.HasPrefix(m.currentView.Title(), "Policy:") || m.currentView.Title() == "RBAC Policy" {
			helpText = " <Esc/Backspace> Go Back | </> Filter Rules"
		} else if m.currentView.Title() == "Deployments" {
			helpText = " <+> Scale Up <-> Scale Dn <r> Restart <e> Edit <d> Describe <x> Delete"
		} else if strings.Contains(m.currentView.Title(), "Services") {
			helpText = " Cmds: <:> Command </> Filter <D>iff <e> Edit <d> Describe <S> Service Heuristic"
		} else if _, ok := m.currentView.(*GenericView); ok {
			helpText = fmt.Sprintf(" GENERIC VIEW: %s | <Enter> Describe <y> YAML <d> Describe <e> Edit <x> Delete <n> Namespace </> Filter <D>iff", m.currentView.Title())
		} else {
			helpText = " Cmds: <:> Command <n> Namespace </> Filter <D>iff <e> Edit <l> Log <x> Delete <d> Describe <F> FwdMgr"
		}

		if m.diffBaseResource != nil {
			helpText = fmt.Sprintf(" \U0001f3af DIFF SOURCE: %s. Select Target and press <D> to compare.", m.diffBaseResource.Name)
		}

		if m.viewingForwards {
			helpText = " [PORT FORWARD MODE] Press <x> to Stop Selected Tunnel | <F> to Return"
		}
		if m.filterInput.Value() != "" {
			helpText += fmt.Sprintf(" [Filter: %s]", m.filterInput.Value())
		}

		
		var footerBar string
		if m.currentView.Title() == "Helm Releases" {
			footerBar = lipgloss.NewStyle().Background(lipgloss.Color("#222222")).Foreground(lipgloss.Color("#FFFFFF")).Width(m.width).Render(helpText)
		} else {
			footerBar = lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Width(m.width).Render(helpText)
		}

		
		if m.statusMsg != "" {
			statusNotification := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")). 
				Background(lipgloss.Color("#333333")). 
				Padding(0, 1).
				Width(m.width).
				Render(fmt.Sprintf("ℹ️  %s", m.statusMsg))

			statusBar = lipgloss.JoinVertical(lipgloss.Left, statusNotification, footerBar)
		} else {
			statusBar = footerBar
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, " ", titleBar, tableContent.String(), statusBar)
}

func (m *ExplorerModel) renderForwardsTable(b *strings.Builder) {
	forwards := m.pfManager.List()
	sort.Slice(forwards, func(i, j int) bool { return forwards[i].LocalPort < forwards[j].LocalPort })
	headers := []string{"POD", "NAMESPACE", "LOCAL -> REMOTE", "STATUS"}
	widths := []int{40, 20, 25, 15}
	headerStyle := styles.TableHeaderStyle
	headerRow := lipgloss.JoinHorizontal(lipgloss.Left,
		headerStyle.Width(widths[0]).Render(headers[0]),
		headerStyle.Width(widths[1]).Render(headers[1]),
		headerStyle.Width(widths[2]).Render(headers[2]),
		headerStyle.Width(widths[3]).Render(headers[3]),
	)
	b.WriteString(headerRow + "\n")
	visibleLines := m.height - 15
	if visibleLines < 1 {
		visibleLines = 1
	}
	start, end := calculateScroll(m.selectedIdx, len(forwards), visibleLines)
	for i := start; i < end; i++ {
		fw := forwards[i]
		rowStyle := styles.DefaultRowStyle
		if i == m.selectedIdx {
			rowStyle = styles.SelectedRowStyle
		}
		status := "Active"
		if !fw.Active {
			status = "Error/Stopped"
		}
		if fw.Error != nil {
			status = "Failed"
		}
		portStr := fmt.Sprintf("%s:%s", fw.LocalPort, fw.RemotePort)
		cells := []string{
			renderCell(fw.PodName, widths[0], rowStyle, lipgloss.Style{}),
			renderCell(fw.Namespace, widths[1], rowStyle, lipgloss.Style{}),
			renderCell(portStr, widths[2], rowStyle, lipgloss.Style{}),
			renderCell(status, widths[3], rowStyle, lipgloss.Style{}),
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, cells...) + "\n")
	}
}
func (m *ExplorerModel) renderResourcesTable(b *strings.Builder) {
	m.clampCursor()
	headers, widths := m.currentView.Headers()
	if m.currentView.Title() == "Events" {
		available := m.width - 4
		wAge := 12
		wType := 12
		wNs := 15
		widths[0] = wAge
		widths[1] = wNs
		widths[2] = wType
		remaining := available - (wAge + wType + wNs)
		if remaining < 10 {
			remaining = 10
		}
		wReason := int(float64(remaining) * 0.15)
		if wReason < 10 {
			wReason = 10
		}
		wObj := int(float64(remaining) * 0.25)
		if wObj < 15 {
			wObj = 15
		}
		wMsg := available - (wAge + wType + wNs + wReason + wObj)
		if wMsg < 20 {
			wMsg = 20
		}
		widths[3] = wReason
		widths[4] = wObj
		widths[5] = wMsg
	}
	headerStyle := styles.TableHeaderStyle
	var headerCells []string
	for i, h := range headers {
		suffix := ""
		key := ""
		lowerH := strings.ToLower(h)
		if strings.Contains(lowerH, "name") {
			key = "name"
		} else if strings.Contains(lowerH, "cpu") {
			key = "cpu"
		} else if strings.Contains(lowerH, "mem") {
			key = "mem"
		} else if strings.Contains(lowerH, "restarts") {
			key = "restarts"
		} else if strings.Contains(lowerH, "age") {
			key = "age"
		}
		if key != "" && m.sortBy == key {
			if m.sortAsc {
				suffix = " \u25b2"
			} else {
				suffix = " \u25bc"
			}
		}
		style := headerStyle.Width(widths[i]).MaxWidth(widths[i])
		headerCells = append(headerCells, style.Render(h+suffix))
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, headerCells...) + "\n")
	visibleLines := m.height - 15
	if visibleLines < 1 {
		visibleLines = 1
	}
	padRemaining := func(currentCount int) {
		if m.currentView.Title() == "Events" {
			if currentCount < visibleLines {
				b.WriteString(strings.Repeat("\n", visibleLines-currentCount))
			}
		}
	}
	if len(m.resources) == 0 && len(m.rawResources) > 0 {
		b.WriteString("No matching resources found for filter.\n")
		padRemaining(1)
		return
	} else if len(m.resources) == 0 {
		b.WriteString("Waiting for events or no resources found...\n")
		padRemaining(1)
		return
	}
	start := m.tableStartIdx
	end := start + visibleLines
	if end > len(m.resources) {
		end = len(m.resources)
	}
	rowsRendered := 0
	for i := start; i < end; i++ {
		if i >= len(m.resources) {
			break
		}
		r := m.resources[i]
		rowStyle := styles.DefaultRowStyle
		if i == m.selectedIdx {
			rowStyle = styles.SelectedRowStyle
		}
		if m.diffBaseResource != nil && m.diffBaseResource.Name == r.Name && m.diffBaseResource.Namespace == r.Namespace {
			rowStyle = rowStyle.Copy().Background(lipgloss.Color("#550000"))
			if i == m.selectedIdx {
				rowStyle = rowStyle.Copy().Background(lipgloss.Color("#880000"))
			}
		}

		var statusColor lipgloss.Color
		s := strings.ToLower(r.Status)
		if strings.Contains(s, "error") || strings.Contains(s, "crash") || strings.Contains(s, "fail") || strings.Contains(s, "image") || strings.Contains(s, "off") || strings.Contains(s, "terminat") || strings.Contains(s, "unhealthy") || strings.Contains(s, "missing") || strings.Contains(s, "evicted") || strings.Contains(s, "notready") {
			statusColor = lipgloss.Color("#FF0000")
		} else if strings.Contains(s, "pending") || strings.Contains(s, "create") || strings.Contains(s, "init") || strings.Contains(s, "warn") || strings.Contains(s, "unknown") || strings.Contains(s, "cordon") {
			statusColor = lipgloss.Color("#FFA500")
		} else if strings.Contains(s, "run") || strings.Contains(s, "ready") || strings.Contains(s, "succ") || strings.Contains(s, "complete") || strings.Contains(s, "active") || strings.Contains(s, "bound") {
			statusColor = lipgloss.Color("#00FF00")
		} else {
			statusColor = lipgloss.Color("#FFFFFF")
		}

		coloredCellStyle := rowStyle.Copy().Foreground(statusColor)
		if i == m.selectedIdx {
			coloredCellStyle = coloredCellStyle.Bold(true)
		}

		if m.currentView.Title() == "Helm Releases" {
			colorHint := r.Extras[6]
			hStatusColor := lipgloss.Color(styles.ColorPending)
			if colorHint == "ok" {
				hStatusColor = lipgloss.Color(styles.ColorRunning)
			}
			if colorHint == "error" {
				hStatusColor = lipgloss.Color(styles.ColorError)
			}

			hStyle := rowStyle.Copy().Foreground(hStatusColor)
			if i == m.selectedIdx {
				hStyle = hStyle.Bold(true)
			}

			cells := []string{
				renderCell(r.Name, widths[0], hStyle, lipgloss.Style{}),
				renderCell(r.Namespace, widths[1], rowStyle, lipgloss.Style{}),
				renderCell(r.Status, widths[2], hStyle, lipgloss.Style{}),
				renderCell(r.Extras[0], widths[3], rowStyle, lipgloss.Style{}),
				renderCell(r.Extras[1], widths[4], rowStyle, lipgloss.Style{}),
				renderCell(r.Extras[2], widths[5], rowStyle, lipgloss.Style{}),
				renderCell(r.Extras[3], widths[6], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))),
			}
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, cells...) + "\n")
			rowsRendered++
			continue
		}
		if m.currentView.Title() == "Events" {
			eventType := strings.TrimSpace(r.Extras[0])
			typeColor := lipgloss.Color(styles.ColorRunning)
			msgColor := lipgloss.Color(styles.ColorDefault)

			if strings.ToLower(eventType) != "normal" {
				typeColor = lipgloss.Color(styles.ColorError)
				msgColor = lipgloss.Color(styles.ColorPending)
			}

			c0 := renderCell(r.Age, widths[0], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")))
			c1 := renderCell(r.Namespace, widths[1], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")))
			c2Style := rowStyle.Copy().Foreground(typeColor).Bold(true)
			c2 := renderCell(eventType, widths[2], c2Style, lipgloss.Style{})
			c3 := renderCell(r.Extras[1], widths[3], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")))
			c4 := renderCell(r.Extras[2], widths[4], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")))
			c5Style := rowStyle.Copy().Foreground(msgColor)
			c5 := renderCell(r.Extras[3], widths[5], c5Style, lipgloss.Style{})

			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, c0, c1, c2, c3, c4, c5) + "\n")
			rowsRendered++
			continue
		}
		if m.currentView.Title() == "Nodes" {
			cells := []string{
				renderCell(r.Name, widths[0], coloredCellStyle, lipgloss.Style{}),
				renderCell(r.Status, widths[1], coloredCellStyle, lipgloss.Style{}),
				renderCell(r.Kind, widths[2], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))),
				renderCell(r.Namespace, widths[3], rowStyle, lipgloss.Style{}),
				renderCell(r.CPU, widths[4], rowStyle, lipgloss.Style{}),
				renderCell(r.Memory, widths[5], rowStyle, lipgloss.Style{}),
				renderCell(r.Age, widths[6], rowStyle, lipgloss.Style{}),
			}
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, cells...) + "\n")
			rowsRendered++
			continue
		}
		var cells []string

		if _, isGeneric := m.currentView.(*GenericView); isGeneric {
			for colIdx := 0; colIdx < len(headers); colIdx++ {
				val := ""
				if colIdx < len(r.Extras) {
					val = r.Extras[colIdx]
				}

				colStyle := rowStyle
				if strings.EqualFold(headers[colIdx], "STATUS") {
					colStyle = coloredCellStyle
				} else if strings.EqualFold(headers[colIdx], "NAME") {
					colStyle = coloredCellStyle
				}

				if colIdx < len(widths) {
					cells = append(cells, renderCell(val, widths[colIdx], colStyle, lipgloss.Style{}))
				}
			}
		} else if m.currentView.Title() == "PersistentVolumeClaims" || m.currentView.Title() == "PersistentVolumes" || m.currentView.Title() == "StorageClasses" {
			cells = append(cells, renderCell(r.Name, widths[0], coloredCellStyle, lipgloss.Style{}))
			extraIdx := 0
			startExtra := 1
			if m.currentView.Title() == "PersistentVolumeClaims" {
				cells = append(cells, renderCell(r.Namespace, widths[1], rowStyle, lipgloss.Style{}))
				cells = append(cells, renderCell(r.Status, widths[2], coloredCellStyle, lipgloss.Style{}))
				startExtra = 3
			} else if m.currentView.Title() == "PersistentVolumes" {
				startExtra = 1
			} else {
				startExtra = 1
			}

			for c := startExtra; c < len(headers)-1; c++ {
				val := ""
				if extraIdx < len(r.Extras) {
					val = r.Extras[extraIdx]
					extraIdx++
				}
				style := rowStyle
				if m.currentView.Title() == "PersistentVolumes" && headers[c] == "STATUS" {
					style = coloredCellStyle
				}
				cells = append(cells, renderCell(val, widths[c], style, lipgloss.Style{}))
			}
			cells = append(cells, renderCell(r.Age, widths[len(widths)-1], rowStyle, lipgloss.Style{}))

		} else if len(headers) >= 10 && headers[2] == "GET" {
			cells = append(cells, renderCell(r.Name, widths[0], rowStyle, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))))
			cells = append(cells, renderCell(r.Kind, widths[1], rowStyle, lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)))
			for j, check := range r.Extras {
				wIndex := 2 + j
				if wIndex < len(widths) {
					checkStyle := lipgloss.NewStyle()
					if check != "" {
						checkStyle = checkStyle.Foreground(lipgloss.Color("#00FF00")).Bold(true)
					}
					content := checkStyle.Render(check)
					centered := lipgloss.PlaceHorizontal(widths[wIndex], lipgloss.Center, content)
					if i == m.selectedIdx {
						centered = styles.SelectedRowStyle.Render(centered)
					} else {
						centered = styles.DefaultRowStyle.Render(centered)
					}
					cells = append(cells, centered)
				}
			}
		} else {
			cells = append(cells, renderCell(r.Name, widths[0], coloredCellStyle, lipgloss.Style{}))

			if len(headers) > 1 {
				cells = append(cells, renderCell(r.Namespace, widths[1], rowStyle, lipgloss.Style{}))
			}
			if len(headers) > 2 {
				cells = append(cells, renderCell(r.Status, widths[2], coloredCellStyle, lipgloss.Style{}))
			}
			if m.currentView.Title() == "Pods" {
				currentCol := 3
				if len(headers) > currentCol {
					cells = append(cells, renderCell(r.CPU, widths[currentCol], rowStyle, lipgloss.Style{}))
					currentCol++
				}
				if len(headers) > currentCol {
					cells = append(cells, renderCell(r.Memory, widths[currentCol], rowStyle, lipgloss.Style{}))
					currentCol++
				}
				if len(headers) > currentCol {
					val := r.Age
					if m.currentView.Title() == "Pods" {
						val = fmt.Sprintf("%d", r.Restarts)
					}
					cells = append(cells, renderCell(val, widths[currentCol], rowStyle, lipgloss.Style{}))
					currentCol++
				}
				if len(headers) > currentCol {
					cells = append(cells, renderCell(r.Age, widths[currentCol], rowStyle, lipgloss.Style{}))
				}
			} else if len(headers) > 3 {
				extraIdx := 0
				for c := 3; c < len(headers)-1; c++ {
					val := ""
					if extraIdx < len(r.Extras) {
						val = r.Extras[extraIdx]
						extraIdx++
					}
					cells = append(cells, renderCell(val, widths[c], rowStyle, lipgloss.Style{}))
				}
				cells = append(cells, renderCell(r.Age, widths[len(widths)-1], rowStyle, lipgloss.Style{}))
			}
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, cells...) + "\n")
		rowsRendered++
	}
	padRemaining(rowsRendered)
}

func calculateScroll(idx, total, visible int) (int, int) {
	start := 0
	if idx >= start+visible {
		start = idx - visible + 1
	} else if idx < start {
		start = idx
	}
	end := start + visible
	if end > total {
		end = total
	}
	return start, end
}
func truncateString(s string, l int) string {
	if lipgloss.Width(s) <= l {
		return s
	}
	if strings.Contains(s, "\x1b") {
		return s
	}
	if len(s) > l {
		return s[:l] + "..."
	}
	return s
}
func renderCell(text string, w int, rowStyle, customStyle lipgloss.Style) string {
	padding := 2
	safeWidth := w - padding
	if safeWidth < 0 {
		safeWidth = 0
	}
	truncated := truncateString(text, safeWidth)
	finalStyle := rowStyle.Copy().Inherit(customStyle)
	return finalStyle.Width(w).MaxWidth(w).MaxHeight(1).Render(truncated)
}



func (m *ExplorerModel) viewYAML(res Resource) (tea.Model, tea.Cmd) {
	
	kind, gvr := m.resolveKind()

	
	if kind == "" && gvr.Resource == "" {
		m.statusMsg = "Error: Unable to identify resource type for YAML view."
		return m, nil
	}


	resourceType := gvr.Resource
	if resourceType == "" {
		resourceType = strings.ToLower(kind) + "s"
	}

	m.statusMsg = fmt.Sprintf("Fetching YAML for %s/%s...", resourceType, res.Name)

	
	cfg := k8syamlviewer.YAMLConfig{
		K8sClient:    m.k8sClient,
		Name:         res.Name,
		Namespace:    res.Namespace,
		ResourceType: resourceType,
	}


	m.yamlViewer = k8syamlviewer.NewYAMLViewer(cfg)
	m.showYAML = true

	return m, m.yamlViewer.Init()
}


func (m *ExplorerModel) resolveKind() (string, schema.GroupVersionResource) {
	switch v := m.currentView.(type) {

	
	case *GenericView:
		return "", v.GetGVR()

	
	case *PodView:
		return "Pod", schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	case *ServiceView:
		return "Service", schema.GroupVersionResource{Version: "v1", Resource: "services"}
	case *NodeView:
		return "Node", schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	case *EventView:
		return "Event", schema.GroupVersionResource{Version: "v1", Resource: "events"}


	case *HelmView:
		return "Secret", schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

	default:
	
		if len(m.resources) > 0 {
			return m.resources[m.selectedIdx].Kind, schema.GroupVersionResource{}
		}
		return "", schema.GroupVersionResource{}
	}
}


func (m *ExplorerModel) resolvePodForForwarding(r Resource) (Resource, error) {
	
	kind := r.Kind
	if kind == "" {
		title := m.currentView.Title()
		if strings.Contains(title, "Service") {
			kind = "Service"
		} else if strings.Contains(title, "Deployment") {
			kind = "Deployment"
		} else if strings.Contains(title, "StatefulSet") {
			kind = "StatefulSet"
		} else if strings.Contains(title, "DaemonSet") {
			kind = "DaemonSet"
		} else if strings.Contains(title, "ReplicaSet") {
			kind = "ReplicaSet"
		}
	}

	
	if kind == "Pod" {
		return r, nil
	}

	
	if kind == "" {
		return Resource{}, fmt.Errorf("could not determine resource kind from view: %s", m.currentView.Title())
	}

	ctx := context.TODO()
	client := m.k8sClient.GetClientset()
	var labelSelector string
	var err error

	
	switch kind {
	case "Service":
		svc, err := client.CoreV1().Services(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			return Resource{}, err
		}
		if len(svc.Spec.Selector) == 0 {
			return Resource{}, fmt.Errorf("service has no selectors (ExternalName or headless?)")
		}
		labelSelector = metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: svc.Spec.Selector})

	case "Deployment":
		dep, err := client.AppsV1().Deployments(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			return Resource{}, err
		}
		labelSelector = metav1.FormatLabelSelector(dep.Spec.Selector)

	case "StatefulSet":
		sts, err := client.AppsV1().StatefulSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			return Resource{}, err
		}
		labelSelector = metav1.FormatLabelSelector(sts.Spec.Selector)

	case "DaemonSet":
		ds, err := client.AppsV1().DaemonSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			return Resource{}, err
		}
		labelSelector = metav1.FormatLabelSelector(ds.Spec.Selector)

	case "ReplicaSet":
		rs, err := client.AppsV1().ReplicaSets(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
		if err != nil {
			return Resource{}, err
		}
		labelSelector = metav1.FormatLabelSelector(rs.Spec.Selector)

	default:
		return Resource{}, fmt.Errorf("unsupported resource for auto-forwarding: %s", kind)
	}

	
	pods, err := client.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return Resource{}, fmt.Errorf("failed to list pods: %v", err)
	}

	
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return Resource{
				Name:      p.Name,
				Namespace: p.Namespace,
				Kind:      "Pod",
			}, nil
		}
	}

	return Resource{}, fmt.Errorf("no running pods found for %s/%s", kind, r.Name)
}

func (m *ExplorerModel) checkConnection() tea.Cmd {
	
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		err := m.k8sClient.IsHealthy()
		return connectionCheckMsg{err: err}
	})
}

type errMsg struct {
	err error
}

func (e errMsg) Error() string {
	return e.err.Error()
}

func (m *ExplorerModel) triggerXRay() (tea.Model, tea.Cmd) {
	if len(m.resources) == 0 {
		return m, nil
	}
	
	selected := m.resources[m.selectedIdx]
	m.statusMsg = fmt.Sprintf("🔍 X-Ray: Scanning topology for %s...", selected.Name)
	


	return m, func() tea.Msg {
		
		defer func() {
			if r := recover(); r != nil { return }
		}()

		
		type result struct {
			g *tree.TopologyGraph
			e error
		}
		ch := make(chan result, 1)

		go func() {
			g, err := tree.BuildXRayGraph(m.k8sClient, selected.Namespace, selected.Kind, selected.Name)
			ch <- result{g, err}
		}()

		select {
		case res := <-ch:
			return xRayReadyMsg{graph: res.g, err: res.e}
		case <-time.After(3 * time.Second):
			return xRayReadyMsg{err: fmt.Errorf("timeout: scan took too long")}
		}
	}
}

func int64Ptr(i int64) *int64 { return &i }