package explorer

import (
	"sort"
	"time"

	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/core/v1"
)


type PodMetricData struct {
	CPU    string
	Memory string
	CPURaw int64
	MemRaw int64
}

type PodView struct {
	lister  listers.PodLister
	metrics map[string]PodMetricData
}

func NewPodView(lister listers.PodLister) *PodView {
	return &PodView{
		lister:  lister,
		metrics: make(map[string]PodMetricData),
	}
}

func (p *PodView) SetMetrics(data map[string]PodMetricData) {
	p.metrics = data
}

func (p *PodView) Title() string {
	return "Pods"
}


func (p *PodView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
}

func (p *PodView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "STATUS", "CPU", "MEM", "RESTARTS", "AGE"}, []int{40, 15, 15, 20, 20, 10, 10}
}

func (p *PodView) Retrieve(namespace string) ([]Resource, error) {
	var pods []*corev1.Pod
	var err error

	if namespace == "" {
		pods, err = p.lister.List(labels.Everything())
	} else {
		pods, err = p.lister.Pods(namespace).List(labels.Everything())
	}

	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, pod := range pods {
		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}

		ts := pod.CreationTimestamp.Time
		if pod.CreationTimestamp.IsZero() {
			ts = time.Now()
		}

		
		var limitCPU, limitMem, reqCPU, reqMem int64
		for _, c := range pod.Spec.Containers {
			limitCPU += c.Resources.Limits.Cpu().MilliValue()
			limitMem += c.Resources.Limits.Memory().Value()
			reqCPU += c.Resources.Requests.Cpu().MilliValue()
			reqMem += c.Resources.Requests.Memory().Value()
		}

		
		key := pod.Namespace + "/" + pod.Name

		cpuDisplay := "-"
		memDisplay := "-"
		var cpuRaw, memRaw int64

		if m, ok := p.metrics[key]; ok {
			cpuRaw = m.CPURaw
			memRaw = m.MemRaw

			
			denomCPU := limitCPU
			if denomCPU == 0 {
				denomCPU = reqCPU
			}
			cpuDisplay = utils.RenderUsageBar(cpuRaw, denomCPU, "%dm")

			memMiB := memRaw / (1024 * 1024)
			limitMiB := limitMem / (1024 * 1024)
			reqMiB := reqMem / (1024 * 1024)

			denomMem := limitMiB
			if denomMem == 0 {
				denomMem = reqMiB
			}
			memDisplay = utils.RenderUsageBar(memMiB, denomMem, "%dMi")
		}

		uiResources = append(uiResources, Resource{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    getPodStatus(pod),
			Age:       utils.ComputeAge(&pod.CreationTimestamp),
			Kind:      "Pod",

			
			CPU:    cpuDisplay,
			Memory: memDisplay,

			
			AgeRaw:    ts,
			Restarts:  restarts,
			CPURaw:    cpuRaw,
			MemoryRaw: memRaw,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}

func getPodStatus(pod *corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			return status.State.Waiting.Reason
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			return status.State.Terminated.Reason
		}
	}
	switch pod.Status.Phase {
	case corev1.PodRunning:
		return "Running"
	case corev1.PodPending:
		return "Pending"
	case corev1.PodFailed:
		return "Failed"
	case corev1.PodSucceeded:
		return "Succeeded"
	default:
		return string(pod.Status.Phase)
	}
}