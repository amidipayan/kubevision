package explorer

import (
	"sort"
	"strings"

	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/core/v1"
)


type NodeMetricData struct {
	CPU    string
	Memory string
	CPURaw int64
	MemRaw int64
}

type NodeView struct {
	lister  listers.NodeLister
	metrics map[string]NodeMetricData
}

func NewNodeView(lister listers.NodeLister) *NodeView {
	return &NodeView{
		lister:  lister,
		metrics: make(map[string]NodeMetricData),
	}
}

func (n *NodeView) SetMetrics(data map[string]NodeMetricData) {
	n.metrics = data
}

func (n *NodeView) Title() string {
	return "Nodes"
}


func (n *NodeView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
}

func (n *NodeView) Headers() ([]string, []int) {
	
	return []string{"NAME", "STATUS", "ROLES", "VERSION", "CPU", "MEM", "AGE"}, []int{30, 20, 20, 15, 20, 20, 10}
}

func (n *NodeView) Retrieve(namespace string) ([]Resource, error) {
	nodes, err := n.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, node := range nodes {
		
		status := getNodeStatus(node)
		if node.Spec.Unschedulable {
			status += " (Cordoned)"
		}

		
		roles := []string{}
		for k := range node.Labels {
			if strings.Contains(k, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
				if role == "" {
					role = "worker"
				} 
				roles = append(roles, role)
			}
		}
		if len(roles) == 0 {
			roles = append(roles, "<none>")
		}
		roleStr := strings.Join(roles, ",")

	
		allocCPU := node.Status.Allocatable.Cpu().MilliValue()
		allocMem := node.Status.Allocatable.Memory().Value()

		cpuDisplay := "-"
		memDisplay := "-"
		var cpuRaw, memRaw int64

		if m, ok := n.metrics[node.Name]; ok {
			cpuRaw = m.CPURaw
			memRaw = m.MemRaw

			
			if allocCPU > 0 {
				cpuDisplay = utils.RenderUsageBar(cpuRaw, allocCPU, "%dm")
			}

			
			if allocMem > 0 {
				memDisplay = utils.RenderUsageBar(memRaw/(1024*1024), allocMem/(1024*1024), "%dMi")
			}
		}

	
		uiResources = append(uiResources, Resource{
			Name:      node.Name,
			Status:    status,
			Age:       utils.ComputeAge(&node.CreationTimestamp),
			
			
			Kind:      roleStr,                              
			Namespace: node.Status.NodeInfo.KubeletVersion, 

			
			CPU:    cpuDisplay,
			Memory: memDisplay,

			
			AgeRaw:    node.CreationTimestamp.Time,
			CPURaw:    cpuRaw,
			MemoryRaw: memRaw,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}

func getNodeStatus(node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}