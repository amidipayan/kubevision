package explorer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type HelmView struct {
	client *client.KubeClient
}

func NewHelmView(c *client.KubeClient) *HelmView {
	return &HelmView{client: c}
}

func (h *HelmView) Title() string {
	return "Helm Releases"
}


func (h *HelmView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
}


func (h *HelmView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "STATUS", "HEALTH", "CHART", "AGE", "ACTION"}, []int{35, 15, 15, 30, 30, 10, 10}
}

func (h *HelmView) Retrieve(namespace string) ([]Resource, error) {
	targetNs := namespace
	if targetNs == "all" || targetNs == "" {
		targetNs = "" 
	}

	cfg, err := h.client.NewHelmConfiguration(targetNs)
	if err != nil {
		return nil, err
	}

	listClient := action.NewList(cfg)
	if targetNs == "" {
		listClient.AllNamespaces = true
	}
	
	
	listClient.StateMask = action.ListDeployed | 
		action.ListFailed | 
		action.ListUninstalling | 
		action.ListSuperseded |
		action.ListPendingInstall |
		action.ListPendingUpgrade |
		action.ListPendingRollback

	results, err := listClient.Run()
	if err != nil {
		return nil, err
	}

	
	
	healthMap := make(map[string]string)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, r := range results {
		
		if r.Info.Status == release.StatusSuperseded || r.Info.Status == release.StatusUninstalling {
			continue
		}

		wg.Add(1)
		go func(rel *release.Release) {
			defer wg.Done()
			
			
			healthStatus := h.analyzeReleaseHealth(rel)
			
			mu.Lock()
			healthMap[rel.Name] = healthStatus
			mu.Unlock()
		}(r)
	}
	wg.Wait()



	var uiResources []Resource
	for _, r := range results {
		
		var ts time.Time
		if r.Info != nil && !r.Info.FirstDeployed.IsZero() {
			ts = r.Info.FirstDeployed.Time
		}
		age := utils.ComputeAge(&metav1.Time{Time: ts})

	
		actionType := "🆕 Install"
		if r.Version > 1 {
			actionType = fmt.Sprintf("🆙 Upgr v%d", r.Version)
		
			if r.Info != nil && strings.Contains(strings.ToLower(r.Info.Description), "rollback") {
				actionType = fmt.Sprintf("⏪ Roll v%d", r.Version)
			}
		}

		
		rawStatus := r.Info.Status.String()
		statusIcon := "○"
		colorHint := "pending"
		
		
		healthDisplay := "❓ Unknown"
		if h, ok := healthMap[r.Name]; ok {
			healthDisplay = h
		} else if rawStatus == "superseded" {
			healthDisplay = "💤 History"
		}

		switch rawStatus {
		case "deployed":
			statusIcon = "🚀"
			colorHint = "ok"
		case "failed":
			statusIcon = "💥"
			colorHint = "error"
			healthDisplay = "💔 Deployment Failed"
		case "pending-install":
			statusIcon = "⏳"
			colorHint = "warn"
			healthDisplay = "⚡ Installing..."
		case "pending-upgrade":
			statusIcon = "⏳"
			colorHint = "warn"
			healthDisplay = "⚡ Upgrading..."
		case "pending-rollback":
			statusIcon = "⏳"
			colorHint = "warn"
			healthDisplay = "⚡ Rolling Back..."
		case "uninstalling":
			statusIcon = "🗑️"
			colorHint = "warn"
			healthDisplay = "💀 Terminating..."
		case "superseded":
			statusIcon = "📜"
			colorHint = "ignore" 
		}

		displayStatus := fmt.Sprintf("%s %s", statusIcon, strings.ToUpper(rawStatus))
		
		
		chartDisplay := "📦 Unknown"
		if r.Chart != nil && r.Chart.Metadata != nil {
			chartDisplay = fmt.Sprintf("📦 %s %s", r.Chart.Metadata.Name, r.Chart.Metadata.Version)
		}

		uiResources = append(uiResources, Resource{
			
			Name:      r.Name,
			Namespace: r.Namespace,
			Status:    displayStatus, 
			Kind:      "HelmRelease",
			Age:       age,
			AgeRaw:    ts,
			
	
			Extras: []string{
				healthDisplay,
				chartDisplay,
				age,
				actionType,
				fmt.Sprintf("%d", r.Version),
				r.Info.Description,
				colorHint,
			},
		})
	}

	
	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}


func (h *HelmView) analyzeReleaseHealth(r *release.Release) string {
	
	objs, err := h.client.ParseManifestToObjects(r.Manifest)
	if err != nil {
		return "⚠️ Manifest Error"
	}

	totalPods := 0
	healthyPods := 0
	
	workloadFound := false

	for _, obj := range objs {
		kind := obj.GetKind()
		if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
			workloadFound = true
			
			
			gvr := getGVR(kind)
			
			
			liveObj, err := h.client.GetDynamicClient().
				Resource(gvr).
				Namespace(r.Namespace).
				Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
			
			if err != nil {
				continue 
			}

			
			specReplicas, foundSpec, _ := unstructured.NestedInt64(liveObj.Object, "spec", "replicas")
			if !foundSpec && kind == "DaemonSet" { specReplicas = 1 } 
			
			statusReplicas, foundStatus, _ := unstructured.NestedInt64(liveObj.Object, "status", "readyReplicas")
			if !foundStatus { statusReplicas = 0 }

			totalPods += int(specReplicas)
			healthyPods += int(statusReplicas)
		}
	}

	if !workloadFound {
		
		pods, err := h.client.GetClientset().CoreV1().Pods(r.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", r.Name),
		})
		
		if err == nil && len(pods.Items) > 0 {
			workloadFound = true
			for _, p := range pods.Items {
				totalPods++
				if p.Status.Phase == corev1.PodRunning && isPodReady(&p) {
					healthyPods++
				}
			}
		}
	}

	if !workloadFound {
		return "❤️  Healthy (Cfg)"
	}

	if totalPods == 0 {
		return "💤 Scaled to 0"
	}

	if healthyPods == totalPods {
		return fmt.Sprintf("❤️  Healthy (%d/%d)", healthyPods, totalPods)
	}

	if healthyPods == 0 {
		return fmt.Sprintf("💔 Broken (0/%d)", totalPods)
	}

	return fmt.Sprintf("⚠️  Degraded (%d/%d)", healthyPods, totalPods)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}


func getGVR(kind string) schema.GroupVersionResource {
	switch kind {
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	case "ReplicaSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	case "Job":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	case "CronJob":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
	default:
		
		return schema.GroupVersionResource{Version: "v1", Resource: strings.ToLower(kind) + "s"}
	}
}