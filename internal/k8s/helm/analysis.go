package helm

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)


type AnalysisResult struct {
	HealthStatus     string
	RiskScore        string
	RiskFactors      []string
	BlastRadius      string
	DriftStatus      string
	DriftDetails     []string 
	SecurityFindings []SecurityFinding
	SecurityCount    int

	
	SecurityProfile SecurityReport

	ManagedResources []ManagedResource
	Events           []corev1.Event
}


type SecurityReport struct {
	Grade        string 
	Score        int    
	ChecksPassed int
	ChecksFailed int
	ChecksTotal  int       
	EvaluatedAt  time.Time 

	
	RiskVelocity string 
	PrevScore    int    

	
	ScoreRBAC      int
	ScoreHardening int
	ScoreImages    int
	ScoreSecrets   int
	ScoreExposure  int
	ScoreSupply    int 

	
	Explainability map[string][]CheckResult
	NextSteps      []string 

	
	Findings []CheckResult

	
	NextBestAction *CheckResult

	
	BlastRadiusInfo BlastRadiusContext
}

type CheckResult struct {
	Passed          bool
	Severity        string 
	Confidence      string 
	Effort          string 
	Category        string
	Message         string
	Why             string 
	Remediation     string 
	RemediationCode string 
	ScoreImpact     int    
}


type BlastRadiusContext struct {
	Score          float64 
	NamespaceLevel string  
	NetworkLevel   string  
	RBACLevel      string  
	PrivilegeLevel string  
}

type ManagedResource struct {
	GVK       schema.GroupVersionKind
	Name      string
	Namespace string
	Status    string
	ColorHint string
	Info      string
	Impact    string
	IsLive    bool

	
	DriftIssues []string
	HasDrift    bool

	
	IsChild  bool
	TreeLine string
}

type SecurityFinding struct {
	Severity   string 
	Title      string
	Suggestion string
	Scope      string 
	Category   string 
}

type Analyzer struct {
	KubeClient *client.KubeClient
}

func NewAnalyzer(kc *client.KubeClient) *Analyzer {
	return &Analyzer{KubeClient: kc}
}


func (a *Analyzer) AnalyzeRelease(rel *release.Release, history []*release.Release) (*AnalysisResult, error) {
	result := &AnalysisResult{
		HealthStatus: "Unknown",
		DriftStatus:  "SYNCED",
	}

	
	objects, err := a.KubeClient.ParseManifestToObjects(rel.Manifest)
	if err != nil {
		return nil, fmt.Errorf("manifest parse failed: %w", err)
	}

	a.analyzeStaticRisk(objects, result)
	result.SecurityProfile = AnalyzeSecurityPosture(rel, objects, history)

	
	result.SecurityCount = result.SecurityProfile.ChecksFailed
	result.RiskScore = fmt.Sprintf("%d (%s)", result.SecurityProfile.Score, result.SecurityProfile.Grade)
	result.BlastRadius = fmt.Sprintf("%0.1f/5.0", result.SecurityProfile.BlastRadiusInfo.Score)


	type resourceResult struct {
		parent   ManagedResource
		children []ManagedResource
	}

	var mu sync.Mutex
	resultsMap := make(map[string]resourceResult) 
	var wg sync.WaitGroup

	
	sem := make(chan struct{}, 20)

	for _, obj := range objects {
		wg.Add(1)
		go func(desired *unstructured.Unstructured) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			targetNs := desired.GetNamespace()
			if targetNs == "" {
				targetNs = rel.Namespace
			}

			live, err := a.fetchLiveResource(desired, targetNs)

			mr := ManagedResource{
				GVK:       desired.GroupVersionKind(),
				Name:      desired.GetName(),
				Namespace: targetNs,
				Status:    "Missing",
				ColorHint: "error",
				Info:      "-",
				IsLive:    false,
			}

			children := []ManagedResource{}

			if err == nil && live != nil {
				mr.IsLive = true
				status, info, ok := a.evaluateHealth(live)
				mr.Status = status
				mr.Info = info
				mr.ColorHint = "ok"
				if !ok {
					mr.ColorHint = "warn"
					
					if isWorkload(desired.GetKind()) {
						mr.ColorHint = "error"
					}
				}

				
				mr.DriftIssues = a.checkDrift(desired, live)
				if len(mr.DriftIssues) > 0 {
					mr.HasDrift = true
				}

				
				if isControlPlane(desired.GetKind()) {
					mr.Impact = "[ADM]"
				} else {
					mr.Impact = getImpactTag(desired.GetKind())
				}

				
				if isWorkload(desired.GetKind()) {
					children = a.fetchChildPods(live)
				}
			} else {
				
				mr.Impact = getImpactTag(desired.GetKind())
			}

			mu.Lock()
			resultsMap[fmt.Sprintf("%s/%s", mr.GVK.Kind, mr.Name)] = resourceResult{
				parent:   mr,
				children: children,
			}
			mu.Unlock()
		}(obj)
	}
	wg.Wait()

	
	var workloads, networking, config, rbac, other []*unstructured.Unstructured
	for _, obj := range objects {
		k := obj.GetKind()
		if isWorkload(k) {
			workloads = append(workloads, obj)
		} else if k == "Service" || k == "Ingress" {
			networking = append(networking, obj)
		} else if k == "ConfigMap" || k == "Secret" {
			config = append(config, obj)
		} else if strings.Contains(k, "Role") || k == "ServiceAccount" {
			rbac = append(rbac, obj)
		} else {
			other = append(other, obj)
		}
	}

	orderedGroups := [][]*unstructured.Unstructured{workloads, networking, config, rbac, other}
	finalList := make([]ManagedResource, 0)

	for _, group := range orderedGroups {
		sort.Slice(group, func(i, j int) bool { return group[i].GetName() < group[j].GetName() })
		for _, obj := range group {
			key := fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())
			res, exists := resultsMap[key]
			if exists {
				
				finalList = append(finalList, res.parent)

				
				if len(res.children) > 0 {
					finalList = append(finalList, res.children...)
				}

				
				if res.parent.HasDrift {
					result.DriftDetails = append(result.DriftDetails, res.parent.DriftIssues...)
				}
				
				if !res.parent.IsLive && res.parent.ColorHint == "error" {
					result.DriftDetails = append(result.DriftDetails, fmt.Sprintf("Missing: %s/%s", obj.GetKind(), obj.GetName()))
				}
			}
		}
	}

	
	storageRes := a.fetchRelatedStorage(rel.Namespace, rel.Name)
	if len(storageRes) > 0 {
		finalList = append(finalList, storageRes...)
	}

	result.ManagedResources = finalList

	
	isHealthy := true
	for _, r := range finalList {
		if r.ColorHint == "error" {
			isHealthy = false
			break
		}
	}

	if rel.Info.Status == release.StatusFailed {
		result.HealthStatus = "Failed"
	} else if !isHealthy {
		result.HealthStatus = "Degraded"
	} else {
		result.HealthStatus = "Healthy"
	}

	if len(result.DriftDetails) > 0 {
		result.DriftStatus = "CONFIG DRIFT"
		for _, d := range result.DriftDetails {
			if strings.Contains(d, "Replicas") || strings.Contains(d, "Image") || strings.Contains(d, "Port") {
				result.DriftStatus = "CRITICAL DRIFT"
				break
			}
		}
	}

	
	events, _ := a.KubeClient.GetClientset().CoreV1().Events(rel.Namespace).List(context.TODO(), metav1.ListOptions{Limit: 100})
	if events != nil {
		filteredEvents := []corev1.Event{}
		for _, e := range events.Items {
			if strings.Contains(e.InvolvedObject.Name, rel.Name) || strings.Contains(rel.Name, "ingress") {
				filteredEvents = append(filteredEvents, e)
			}
		}
		result.Events = filteredEvents
	}

	return result, nil
}


func (a *Analyzer) checkDrift(desired, live *unstructured.Unstructured) []string {
	var drifts []string
	kind := desired.GetKind()

	
	if kind == "Deployment" || kind == "StatefulSet" {
		dRep, foundD, _ := unstructured.NestedInt64(desired.Object, "spec", "replicas")
		lRep, foundL, _ := unstructured.NestedInt64(live.Object, "spec", "replicas")
		if foundD && foundL && dRep != lRep {
			drifts = append(drifts, fmt.Sprintf("Replica Mismatch (Want %d, Has %d)", dRep, lRep))
		}
	}

	
	dContainers, foundD, _ := unstructured.NestedSlice(desired.Object, "spec", "template", "spec", "containers")
	lContainers, foundL, _ := unstructured.NestedSlice(live.Object, "spec", "template", "spec", "containers")

	if foundD && foundL {
		dMap := make(map[string]map[string]interface{})
		for _, c := range dContainers {
			if cmap, ok := c.(map[string]interface{}); ok {
				if cName, _, _ := unstructured.NestedString(cmap, "name"); cName != "" {
					dMap[cName] = cmap
				}
			}
		}

		for _, lc := range lContainers {
			lMap, ok := lc.(map[string]interface{})
			if !ok {
				continue
			}
			cName, _, _ := unstructured.NestedString(lMap, "name")
			if dContainer, exists := dMap[cName]; exists {
				
				dImg, _, _ := unstructured.NestedString(dContainer, "image")
				lImg, _, _ := unstructured.NestedString(lMap, "image")
				if dImg != "" && dImg != lImg {
					drifts = append(drifts, fmt.Sprintf("[%s] Image Drift (%s -> %s)", cName, dImg, lImg))
				}

				
				dEnv, _, _ := unstructured.NestedSlice(dContainer, "env")
				lEnv, _, _ := unstructured.NestedSlice(lMap, "env")
				if envDiff := compareEnvVars(dEnv, lEnv); envDiff != "" {
					drifts = append(drifts, fmt.Sprintf("[%s] Env Drift (%s)", cName, envDiff))
				}
			}
		}
	}

	
	if kind == "Service" {
		dPorts, foundD, _ := unstructured.NestedSlice(desired.Object, "spec", "ports")
		lPorts, foundL, _ := unstructured.NestedSlice(live.Object, "spec", "ports")
		if foundD && foundL {
			if portDiff := comparePorts(dPorts, lPorts); portDiff != "" {
				drifts = append(drifts, fmt.Sprintf("Port Mismatch (%s)", portDiff))
			}
		}
	}

	return drifts
}



func compareEnvVars(desired, live []interface{}) string {
	dMap := make(map[string]string)
	lMap := make(map[string]string)

	for _, e := range desired {
		if em, ok := e.(map[string]interface{}); ok {
			name, _, _ := unstructured.NestedString(em, "name")
			val, _, _ := unstructured.NestedString(em, "value")
			if name != "" {
				dMap[name] = val
			}
		}
	}
	for _, e := range live {
		if em, ok := e.(map[string]interface{}); ok {
			name, _, _ := unstructured.NestedString(em, "name")
			val, _, _ := unstructured.NestedString(em, "value")
			if name != "" {
				lMap[name] = val
			}
		}
	}

	var diffs []string
	for k, v := range dMap {
		if lv, exists := lMap[k]; !exists {
			diffs = append(diffs, fmt.Sprintf("-%s", k))
		} else if v != lv {
			diffs = append(diffs, fmt.Sprintf("%s(val changed)", k))
		}
	}
	for k := range lMap {
		if _, exists := dMap[k]; !exists {
			diffs = append(diffs, fmt.Sprintf("+%s", k))
		}
	}
	if len(diffs) > 0 {
		return strings.Join(diffs, ", ")
	}
	return ""
}

func comparePorts(desired, live []interface{}) string {
	
	dMap := make(map[int64]int64) 
	lMap := make(map[int64]int64)

	for _, p := range desired {
		pm, _ := p.(map[string]interface{})
		port, _, _ := unstructured.NestedInt64(pm, "port")
		tPort, _, _ := unstructured.NestedInt64(pm, "targetPort")
		if port != 0 {
			dMap[port] = tPort
		}
	}
	for _, p := range live {
		pm, _ := p.(map[string]interface{})
		port, _, _ := unstructured.NestedInt64(pm, "port")
		tPort, _, _ := unstructured.NestedInt64(pm, "targetPort")
		if port != 0 {
			lMap[port] = tPort
		}
	}

	if !reflect.DeepEqual(dMap, lMap) {
		return "Ports Changed"
	}
	return ""
}

func (a *Analyzer) analyzeStaticRisk(objects []*unstructured.Unstructured, res *AnalysisResult) {
	riskScore := 0
	res.BlastRadius = "Namespace (Local)"

	for _, obj := range objects {
		kind := obj.GetKind()
		name := obj.GetName()

		
		if kind == "ClusterRole" || kind == "ClusterRoleBinding" {
			res.BlastRadius = "Cluster-Wide (High Impact)"
			riskScore += 3
			res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
				Severity: "HIGH", Title: fmt.Sprintf("Cluster-Wide Access (%s)", kind),
				Suggestion: fmt.Sprintf("Verify necessity of cluster scope for %s", name),
				Scope:      "Cluster", Category: "RBAC",
			})
		}

		
		if kind == "MutatingWebhookConfiguration" || kind == "ValidatingWebhookConfiguration" {
			res.BlastRadius = "Cluster Critical (API Blocking)"
			riskScore += 4
			res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
				Severity: "CRITICAL", Title: "Admission Webhook Risk",
				Suggestion: fmt.Sprintf("Ensure %s has failurePolicy='Ignore' or 100%% uptime.", name),
				Scope:      "Cluster", Category: "Admission",
			})
		}

		
		if kind == "Role" || kind == "ClusterRole" {
			rules, found, _ := unstructured.NestedSlice(obj.Object, "rules")
			if found {
				for _, r := range rules {
					rule := r.(map[string]interface{})
					verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
					resources, _, _ := unstructured.NestedStringSlice(rule, "resources")

					hasStarVerb := false
					for _, v := range verbs {
						if v == "*" {
							hasStarVerb = true
						}
					}

					hasSecret := false
					for _, res := range resources {
						if res == "secrets" {
							hasSecret = true
						}
					}

					if hasStarVerb {
						res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
							Severity: "HIGH", Title: "Excessive Permissions (Wildcard Verb)",
							Suggestion: fmt.Sprintf("Remove '*' verb in %s to follow least privilege.", name),
							Scope:      "Namespace", Category: "RBAC",
						})
						riskScore += 2
					}
					if hasSecret && hasStarVerb {
						res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
							Severity: "CRITICAL", Title: "Unrestricted Secret Access",
							Suggestion: "Explicitly list verbs for Secrets to prevent credential theft.",
							Scope:      "Namespace", Category: "RBAC",
						})
						riskScore += 5
					}
				}
			}
		}

		
		if isWorkload(kind) {
			containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			if found {
				for _, c := range containers {
					cmap, _ := c.(map[string]interface{})
					
					if priv, _, _ := unstructured.NestedBool(cmap, "securityContext", "privileged"); priv {
						res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
							Severity: "CRITICAL", Title: "Privileged Container Detected",
							Suggestion: fmt.Sprintf("Disable privileged mode in %s unless strictly required for infrastructure.", name),
							Scope:      "Namespace", Category: "Workload",
						})
						riskScore += 5
					}
					
					if runAs, found, _ := unstructured.NestedInt64(cmap, "securityContext", "runAsUser"); found && runAs == 0 {
						res.SecurityFindings = append(res.SecurityFindings, SecurityFinding{
							Severity: "MEDIUM", Title: "Running as Root",
							Suggestion: fmt.Sprintf("Set runAsUser > 1000 in %s for better isolation.", name),
							Scope:      "Namespace", Category: "Workload",
						})
						riskScore += 1
					}
				}
			}
		}
	}

	res.SecurityCount = len(res.SecurityFindings)
	if res.SecurityCount > 0 {
		riskScore += res.SecurityCount
	}

	
	if riskScore >= 10 {
		res.RiskScore = "CRITICAL"
	} else if riskScore >= 6 {
		res.RiskScore = "HIGH"
	} else if riskScore >= 3 {
		res.RiskScore = "MEDIUM"
	} else {
		res.RiskScore = "LOW"
	}
}

func isControlPlane(kind string) bool {
	return kind == "MutatingWebhookConfiguration" || kind == "ValidatingWebhookConfiguration" || kind == "CustomResourceDefinition" || kind == "APIService"
}

func getImpactTag(kind string) string {
	switch kind {
	case "Service", "Ingress":
		return "[NET]"
	case "PersistentVolumeClaim":
		return "[VOL]"
	case "Secret", "ConfigMap":
		return "[CFG]"
	case "Deployment", "StatefulSet":
		return "[WORKLOAD]"
	case "ClusterRole", "RoleBinding", "ClusterRoleBinding":
		return "[RBAC]"
	case "ValidatingWebhookConfiguration", "MutatingWebhookConfiguration":
		return "[ADM]"
	default:
		return ""
	}
}

func isWorkload(kind string) bool {
	return kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet"
}

func (a *Analyzer) fetchLiveResource(obj *unstructured.Unstructured, ns string) (*unstructured.Unstructured, error) {
	gvk := obj.GroupVersionKind()
	mapping, err := a.KubeClient.GetMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		dri = a.KubeClient.GetDynamicClient().Resource(mapping.Resource).Namespace(ns)
	} else {
		dri = a.KubeClient.GetDynamicClient().Resource(mapping.Resource)
	}
	return dri.Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
}

func (a *Analyzer) evaluateHealth(obj *unstructured.Unstructured) (string, string, bool) {
	kind := obj.GetKind()
	status := "Synced"
	info := ""
	healthy := true

	switch kind {
	case "Pod":
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		status = phase
		if phase != "Running" && phase != "Succeeded" {
			healthy = false
		}
	case "Deployment", "StatefulSet", "DaemonSet":
		replicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
		status = fmt.Sprintf("%d/%d Ready", ready, replicas)
		if ready < replicas {
			healthy = false
		}
	case "Service":
		t, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
		info = t
	case "PersistentVolumeClaim":
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		status = phase
		if phase != "Bound" {
			healthy = false
		}
	}
	return status, info, healthy
}

func (a *Analyzer) fetchChildPods(parent *unstructured.Unstructured) []ManagedResource {
	var children []ManagedResource


	selectorMap, found, _ := unstructured.NestedStringMap(parent.Object, "spec", "selector", "matchLabels")
	if !found || len(selectorMap) == 0 {
		return children
	}

	
	ls := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: selectorMap})

	pods, err := a.KubeClient.GetClientset().CoreV1().Pods(parent.GetNamespace()).List(context.TODO(), metav1.ListOptions{LabelSelector: ls})
	if err != nil {
		return children
	}

	for i, p := range pods.Items {
		status := string(p.Status.Phase)
		color := "ok"
		if status != "Running" && status != "Succeeded" {
			color = "warn"
			if status == "Failed" || status == "Unknown" {
				color = "error"
			}
		}

		
		treeLine := "├─"
		if i == len(pods.Items)-1 {
			treeLine = "└─"
		}

		children = append(children, ManagedResource{
			GVK:       schema.GroupVersionKind{Kind: "Pod"},
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    status,
			ColorHint: color,
			Info:      p.Status.PodIP, 
			IsChild:   true,
			TreeLine:  treeLine,
			IsLive:    true,
			Impact:    "", 
		})
	}
	return children
}


func (a *Analyzer) fetchRelatedStorage(namespace, releaseName string) []ManagedResource {
	var resources []ManagedResource


	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName)
	pvcs, err := a.KubeClient.GetClientset().CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return resources
	}

	for _, pvc := range pvcs.Items {
		
		resources = append(resources, ManagedResource{
			GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Status:    string(pvc.Status.Phase),
			ColorHint: getColorForPhase(string(pvc.Status.Phase)),
			Info:      fmt.Sprintf("%s (%s)", *pvc.Spec.StorageClassName, pvc.Status.Capacity.Storage().String()),
			IsChild:   true, 
			IsLive:    true,
			TreeLine:  "├─",
			Impact:    "[VOL]",
		})

		
		if pvc.Spec.VolumeName != "" {
			pv, err := a.KubeClient.GetClientset().CoreV1().PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
			if err == nil {
				resources = append(resources, ManagedResource{
					GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"},
					Name:      pv.Name,
					Namespace: "-", // Cluster Scoped
					Status:    string(pv.Status.Phase),
					ColorHint: getColorForPhase(string(pv.Status.Phase)),
					Info:      "Bound Physical Storage",
					IsChild:   true,
					IsLive:    true,
					TreeLine:  "│  └─", 
					Impact:    "[DISK]",
				})
			}
		}

		
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			
			resources = append(resources, ManagedResource{
				GVK:       schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"},
				Name:      *pvc.Spec.StorageClassName,
				Namespace: "-",
				Status:    "Active",
				ColorHint: "ok",
				Info:      "Storage Provisioner",
				IsChild:   true,
				IsLive:    true,
				TreeLine:  "│  └─",
				Impact:    "[SC]",
			})
		}
	}

	return resources
}


func getColorForPhase(phase string) string {
	switch phase {
	case "Bound", "Available", "Active":
		return "ok"
	case "Pending", "Released":
		return "warn"
	default:
		return "error"
	}
}