package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)



func BuildXRayGraph(k8s *client.KubeClient, ns, kind, name string) (*TopologyGraph, error) {
	rootID := fmt.Sprintf("%s/%s", kind, name)
	graph := NewTopologyGraph(rootID)
	ctx := context.TODO()


	root := &TopologyNode{
		ID:          rootID,
		Name:        name,
		Namespace:   ns,
		Kind:        mapKindToNodeKind(kind),
		Icon:        getIconForKind(kind),
		Status:      "Active",
		Level:       LevelRoot, 
		Criticality: TierStandard,
	}
	graph.AddNode(root)

	
	switch kind {
	case "Service":
		if err := expandServiceXRay(ctx, k8s, graph, root, ns, name); err != nil {
			return nil, err
		}

	case "Deployment", "StatefulSet", "DaemonSet":
		if err := expandWorkloadXRay(ctx, k8s, graph, root, ns, kind, name); err != nil {
			return nil, err
		}
	
	case "Pod":
		if err := expandPodXRay(ctx, k8s, graph, root, ns, name); err != nil {
			return nil, err
		}
	}


	condenseNoisyNodes(graph, rootID)

	return graph, nil
}


func expandWorkloadXRay(ctx context.Context, k8s *client.KubeClient, g *TopologyGraph, root *TopologyNode, ns, kind, name string) error {
	var selectorStr string
	var podSpec corev1.PodSpec
	var workloadLabels map[string]string 


	switch kind {
	case "Deployment":
		obj, err := k8s.GetClientset().AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
			podSpec = obj.Spec.Template.Spec
			workloadLabels = obj.Spec.Template.Labels 
		}
	case "StatefulSet":
		obj, err := k8s.GetClientset().AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
			podSpec = obj.Spec.Template.Spec
			workloadLabels = obj.Spec.Template.Labels 
		}
	case "DaemonSet":
		obj, err := k8s.GetClientset().AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			selectorStr = metav1.FormatLabelSelector(obj.Spec.Selector)
			podSpec = obj.Spec.Template.Spec
			workloadLabels = obj.Spec.Template.Labels 
		}
	}


	scanForServices(ctx, k8s, g, root.ID, ns, workloadLabels)

	
	scanPodSpecForConfigs(g, root.ID, podSpec)


	if selectorStr != "" {
		pods, err := k8s.GetClientset().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selectorStr})
		if err == nil && len(pods.Items) > 0 {
			
			for i, p := range pods.Items {
				if i >= 5 { break }
				
				podID := fmt.Sprintf("Pod/%s", p.Name)
				g.AddNode(&TopologyNode{ID: podID, Name: p.Name, Kind: KindWorkload, Icon: "📦", Level: LevelCritical})
				g.AddEdge(root.ID, podID, EdgeOwns)

				
				if p.Spec.NodeName != "" {
					nodeID := fmt.Sprintf("Node/%s", p.Spec.NodeName)
					if _, exists := g.Nodes[nodeID]; !exists {
						g.AddNode(&TopologyNode{ID: nodeID, Name: p.Spec.NodeName, Kind: KindInfra, Icon: "💻", Level: LevelCritical})
					}
					g.AddEdge(podID, nodeID, EdgeUses)
				}

				
				scanPodVolumes(ctx, k8s, g, podID, ns, p.Spec.Volumes)
			}
		}
	}

	return nil
}


func expandServiceXRay(ctx context.Context, k8s *client.KubeClient, g *TopologyGraph, root *TopologyNode, ns, name string) error {
	
	ings, err := k8s.GetClientset().NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ing := range ings.Items {
			isRelated := false
			if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil && ing.Spec.DefaultBackend.Service.Name == name {
				isRelated = true
			}
			for _, rule := range ing.Spec.Rules {
				if rule.HTTP == nil { continue }
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil && path.Backend.Service.Name == name {
						isRelated = true
						break
					}
				}
			}

			if isRelated {
				ingID := fmt.Sprintf("Ingress/%s", ing.Name)
				g.AddNode(&TopologyNode{ID: ingID, Name: ing.Name, Kind: KindNetwork, Icon: "🌐", Criticality: TierHigh, Level: LevelCritical})
				g.AddEdge(ingID, root.ID, EdgeExposedBy)
				
				
				g.AddNode(&TopologyNode{ID: "Ext/Internet", Name: "Internet", Kind: KindExternal, Icon: "☁️", Level: LevelPeripheral})
				g.AddEdge("Ext/Internet", ingID, EdgeUses)
			}
		}
	}

	
	svc, err := k8s.GetClientset().CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return err }

	if len(svc.Spec.Selector) > 0 {
		selector := labels.Set(svc.Spec.Selector).String()
		pods, err := k8s.GetClientset().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		
		if err == nil {
			owners := make(map[string]string)
			for _, p := range pods.Items {
				if len(p.OwnerReferences) > 0 {
					own := p.OwnerReferences[0]
					
					if own.Kind == "ReplicaSet" {
						rs, _ := k8s.GetClientset().AppsV1().ReplicaSets(ns).Get(ctx, own.Name, metav1.GetOptions{})
						if rs != nil && len(rs.OwnerReferences) > 0 {
							grandParent := rs.OwnerReferences[0]
							owners[fmt.Sprintf("%s/%s", grandParent.Kind, grandParent.Name)] = grandParent.Kind
							continue
						}
					}
					owners[fmt.Sprintf("%s/%s", own.Kind, own.Name)] = own.Kind
				}
			}

			for id, kind := range owners {
				parts := strings.Split(id, "/")
				g.AddNode(&TopologyNode{ID: id, Name: parts[1], Kind: KindWorkload, Icon: getIconForKind(kind), Level: LevelCritical})
				g.AddEdge(root.ID, id, EdgeUses)
				
				
				expandWorkloadXRay(ctx, k8s, g, g.Nodes[id], ns, kind, parts[1])
			}
		}
	}
	return nil
}

func expandPodXRay(ctx context.Context, k8s *client.KubeClient, g *TopologyGraph, root *TopologyNode, ns, name string) error {
	pod, err := k8s.GetClientset().CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return err }
	
	
	scanPodSpecForConfigs(g, root.ID, pod.Spec)
	
	
	if pod.Spec.NodeName != "" {
		nodeID := fmt.Sprintf("Node/%s", pod.Spec.NodeName)
		g.AddNode(&TopologyNode{ID: nodeID, Name: pod.Spec.NodeName, Kind: KindInfra, Icon: "💻", Level: LevelCritical})
		g.AddEdge(root.ID, nodeID, EdgeUses)
	}

	
	scanPodVolumes(ctx, k8s, g, root.ID, ns, pod.Spec.Volumes)

	return nil
}



func scanForServices(ctx context.Context, k8s *client.KubeClient, g *TopologyGraph, targetID, ns string, targetLabels map[string]string) {
	if len(targetLabels) == 0 { return }
	targetSet := labels.Set(targetLabels)

	svcs, _ := k8s.GetClientset().CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	for _, s := range svcs.Items {
		if len(s.Spec.Selector) == 0 { continue }
		selector := labels.SelectorFromSet(s.Spec.Selector)
		if selector.Matches(targetSet) {
			svcID := fmt.Sprintf("Service/%s", s.Name)
			g.AddNode(&TopologyNode{ID: svcID, Name: s.Name, Kind: KindService, Icon: "🔌", Level: LevelCritical})
			g.AddEdge(svcID, targetID, EdgeUses)
		}
	}
}

func scanPodSpecForConfigs(g *TopologyGraph, parentID string, spec corev1.PodSpec) {
	
	if spec.ServiceAccountName != "" && spec.ServiceAccountName != "default" {
		saID := fmt.Sprintf("ServiceAccount/%s", spec.ServiceAccountName)
		g.AddNode(&TopologyNode{ID: saID, Name: spec.ServiceAccountName, Kind: KindConfig, Icon: "👤", Level: LevelPeripheral})
		g.AddEdge(parentID, saID, EdgeUses)
	}

	
	for _, c := range spec.Containers {
		for _, env := range c.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					addDep(g, parentID, "ConfigMap", env.ValueFrom.ConfigMapKeyRef.Name, "📝", LevelPeripheral)
				}
				if env.ValueFrom.SecretKeyRef != nil {
					addDep(g, parentID, "Secret", env.ValueFrom.SecretKeyRef.Name, "🔒", LevelPeripheral)
				}
			}
		}
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				addDep(g, parentID, "ConfigMap", envFrom.ConfigMapRef.Name, "📝", LevelPeripheral)
			}
			if envFrom.SecretRef != nil {
				addDep(g, parentID, "Secret", envFrom.SecretRef.Name, "🔒", LevelPeripheral)
			}
		}
	}
}

func scanPodVolumes(ctx context.Context, k8s *client.KubeClient, g *TopologyGraph, parentID, ns string, volumes []corev1.Volume) {
	for _, vol := range volumes {
		if vol.PersistentVolumeClaim != nil {
			pvcName := vol.PersistentVolumeClaim.ClaimName
			pvcID := fmt.Sprintf("PVC/%s", pvcName)
			
			
			g.AddNode(&TopologyNode{ID: pvcID, Name: pvcName, Kind: KindInfra, Icon: "💾", Criticality: TierCritical, Level: LevelCritical})
			g.AddEdge(parentID, pvcID, EdgeUses)

			
			pvc, err := k8s.GetClientset().CoreV1().PersistentVolumeClaims(ns).Get(ctx, pvcName, metav1.GetOptions{})
			if err == nil && pvc.Spec.VolumeName != "" {
				pvName := pvc.Spec.VolumeName
				pvID := fmt.Sprintf("PV/%s", pvName)
				g.AddNode(&TopologyNode{ID: pvID, Name: pvName, Kind: KindInfra, Icon: "💽", Level: LevelCritical})
				g.AddEdge(pvcID, pvID, EdgeUses) 

				
				scName := pvc.Spec.StorageClassName
				if scName != nil && *scName != "" {
					scID := fmt.Sprintf("StorageClass/%s", *scName)
					g.AddNode(&TopologyNode{ID: scID, Name: *scName, Kind: KindInfra, Icon: "🗄️", Level: LevelCritical})
					g.AddEdge(pvID, scID, EdgeUses) // PV -> StorageClass
				}
			}
		}
		if vol.ConfigMap != nil {
			addDep(g, parentID, "ConfigMap", vol.ConfigMap.Name, "📝", LevelPeripheral)
		}
		if vol.Secret != nil {
			addDep(g, parentID, "Secret", vol.Secret.SecretName, "🔒", LevelPeripheral)
		}
	}
}


func addDep(g *TopologyGraph, parentID, kind, name, icon string, level VisualLevel) {
	id := fmt.Sprintf("%s/%s", kind, name)
	if _, exists := g.Nodes[id]; !exists {
		g.AddNode(&TopologyNode{ID: id, Name: name, Kind: mapKindToNodeKind(kind), Icon: icon, Level: level})
	}
	g.AddEdge(parentID, id, EdgeConfiguredBy)
}


func condenseNoisyNodes(g *TopologyGraph, rootID string) {
	var configs []string
	
	
	for _, edge := range g.Edges {
		if edge.FromID == rootID {
			child := g.Nodes[edge.ToID]
			if child.Kind == KindConfig {
				configs = append(configs, child.ID)
			}
		}
	}

	
	if len(configs) > 3 {
		
		newEdges := make([]TopologyEdge, 0)
		for _, edge := range g.Edges {
			keep := true
			for _, cID := range configs {
				if edge.ToID == cID {
					keep = false
					break
				}
			}
			if keep {
				newEdges = append(newEdges, edge)
			}
		}
		g.Edges = newEdges

		
		groupID := "Group/Configs"
		g.AddNode(&TopologyNode{
			ID: groupID, Name: "Configs", Kind: KindGroup, 
			Icon: "📚", Level: LevelPeripheral, IsGroup: true, GroupCount: len(configs),
		})
		g.AddEdge(rootID, groupID, EdgeConfiguredBy)
	}
}

func mapKindToNodeKind(k string) NodeKind {
	switch k {
	case "Service": return KindService
	case "Ingress": return KindNetwork
	case "Deployment", "StatefulSet", "DaemonSet", "Pod": return KindWorkload
	case "ConfigMap", "Secret", "ServiceAccount": return KindConfig
	case "Node", "PersistentVolume", "PersistentVolumeClaim", "StorageClass": return KindInfra
	default: return KindUnknown
	}
}

func getIconForKind(k string) string {
	switch k {
	case "Service": return "🔌"
	case "Ingress": return "🌐"
	case "Deployment": return "🚀"
	case "StatefulSet": return "🏢"
	case "DaemonSet": return "👻"
	case "ReplicaSet": return "👯"
	case "Pod": return "📦"
	case "ConfigMap": return "📝"
	case "Secret": return "🔒"
	case "ServiceAccount": return "👤"
	case "PersistentVolumeClaim": return "💾"
	case "PersistentVolume": return "💽"
	case "StorageClass": return "🗄️"
	case "Node": return "💻"
	default: return "🔹"
	}
}

func BuildTopology(k8s *client.KubeClient, ns, kind, name string) (*Node, error) {
	switch kind {
	case "Pod": return buildPodTopology(k8s, ns, name)
	case "Service": return buildServiceTopology(k8s, ns, name)
	case "Deployment": return buildDeploymentTopology(k8s, ns, name)
	case "ReplicaSet": return buildReplicaSetTopology(k8s, ns, name)
	case "StatefulSet": return buildStatefulSetTopology(k8s, ns, name)
	case "DaemonSet": return buildDaemonSetTopology(k8s, ns, name)
	case "ConfigMap": return buildConfigMapTopology(k8s, ns, name)
	case "Secret": return buildSecretTopology(k8s, ns, name)
	default: return NewNode(kind, name, "❓", "No Graph Support"), nil
	}
}


func buildPodTopology(k8s *client.KubeClient, ns, name string) (*Node, error) {
	ctx := context.TODO()
	pod, err := k8s.GetClientset().CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return nil, err }
	root := NewNode("Pod", pod.Name, "📦", string(pod.Status.Phase))
	if len(pod.Spec.Volumes) > 0 {
		vNode := NewVirtualNode("Volumes", "💾")
		for _, v := range pod.Spec.Volumes {
			if v.ConfigMap != nil { vNode.AddChild(NewNode("ConfigMap", v.ConfigMap.Name, "📝", "Mounted")) }
			if v.Secret != nil { vNode.AddChild(NewNode("Secret", v.Secret.SecretName, "🔒", "Mounted")) }
			if v.PersistentVolumeClaim != nil { vNode.AddChild(NewNode("PVC", v.PersistentVolumeClaim.ClaimName, "💽", "Bound")) }
		}
		root.AddChild(vNode)
	}
	return root, nil
}
func buildServiceTopology(k8s *client.KubeClient, ns, name string) (*Node, error) {
	ctx := context.TODO()
	svc, err := k8s.GetClientset().CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return nil, err }
	root := NewNode("Service", svc.Name, "🔌", string(svc.Spec.Type))
	if len(svc.Spec.Selector) > 0 {
		selector := labels.Set(svc.Spec.Selector).String()
		pods, _ := k8s.GetClientset().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		pGroup := NewVirtualNode(fmt.Sprintf("Pods (%d)", len(pods.Items)), "📦")
		for _, p := range pods.Items { pGroup.AddChild(NewNode("Pod", p.Name, "📦", string(p.Status.Phase))) }
		root.AddChild(pGroup)
	}
	return root, nil
}
func buildDeploymentTopology(k8s *client.KubeClient, ns, name string) (*Node, error) {
	ctx := context.TODO()
	dep, err := k8s.GetClientset().AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return nil, err }
	root := NewNode("Deployment", dep.Name, "🚀", fmt.Sprintf("%d/%d", dep.Status.ReadyReplicas, *dep.Spec.Replicas))
	selector, _ := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	rss, _ := k8s.GetClientset().AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	for _, rs := range rss.Items {
		if metav1.IsControlledBy(&rs, dep) {
			rsNode := NewNode("ReplicaSet", rs.Name, "👯", fmt.Sprintf("%d", rs.Status.Replicas))
			rsSelector, _ := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
			pods, _ := k8s.GetClientset().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: rsSelector.String()})
			for _, p := range pods.Items {
				if metav1.IsControlledBy(&p, &rs) { rsNode.AddChild(NewNode("Pod", p.Name, "📦", string(p.Status.Phase))) }
			}
			root.AddChild(rsNode)
		}
	}
	return root, nil
}
func buildReplicaSetTopology(k8s *client.KubeClient, ns, name string) (*Node, error) { return NewNode("ReplicaSet", name, "👯", "See Deployment"), nil }
func buildStatefulSetTopology(k8s *client.KubeClient, ns, name string) (*Node, error) { return NewNode("StatefulSet", name, "🏢", "Stateful Workload"), nil }
func buildDaemonSetTopology(k8s *client.KubeClient, ns, name string) (*Node, error) { return NewNode("DaemonSet", name, "👻", "Daemon Workload"), nil }
func buildConfigMapTopology(k8s *client.KubeClient, ns, name string) (*Node, error) { return NewNode("ConfigMap", name, "📝", "Configuration"), nil }
func buildSecretTopology(k8s *client.KubeClient, ns, name string) (*Node, error) { return NewNode("Secret", name, "🔒", "Sensitive Data"), nil }