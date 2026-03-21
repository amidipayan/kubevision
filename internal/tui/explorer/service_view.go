package explorer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/k8s/sre"
	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)


type ShowServiceAnalysisMsg struct {
	Service string
	Profile sre.SREProfile
}

type ServiceView struct {
	client        *client.KubeClient
	analysisCache map[string]sre.SREProfile
}

func NewServiceView(c *client.KubeClient) *ServiceView {
	return &ServiceView{
		client:        c,
		analysisCache: make(map[string]sre.SREProfile),
	}
}


func (s *ServiceView) Title() string {
	return "Services (SRE Intelligence)"
}


func (s *ServiceView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
}


func (s *ServiceView) ExtraHelp() map[string]string {
	return map[string]string{
		"S": "SRE Deep Scan",
	}
}


func (s *ServiceView) HandleKey(msg tea.KeyMsg, r Resource) (bool, tea.Cmd) {
	if msg.String() == "S" {
		
		key := fmt.Sprintf("%s/%s", r.Namespace, r.Name)
		if profile, ok := s.analysisCache[key]; ok {
			return true, func() tea.Msg {
				return ShowServiceAnalysisMsg{
					Service: r.Name,
					Profile: profile,
				}
			}
		}
	}
	return false, nil
}


func (s *ServiceView) Headers() ([]string, []int) {
	
	return []string{
		"NAME", "NAMESPACE", "TIER", "GRADE", "TYPE", "CLUSTER-IP", "PORTS", "SCORE", "AGE",
	}, []int{
		30, 15, 12, 8, 12, 15, 20, 8, 10,
	}
}


func (s *ServiceView) Retrieve(namespace string) ([]Resource, error) {
	ctx := context.TODO()
	clientset := s.client.GetClientset()


	svcList, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	ingList, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	npList, err := clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	
	depList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	
	s.analysisCache = make(map[string]sre.SREProfile)
	var uiResources []Resource

	
	for _, svc := range svcList.Items {
		
		myPods := s.filterPods(podList.Items, svc.Spec.Selector)
		myIngresses := s.filterIngresses(ingList.Items, svc.Name)
		
	
		myNetPols := s.filterNetPols(npList.Items, svc.Namespace)

		
		profile := sre.AnalyzeService(clientset, svc, myPods, myIngresses, myNetPols, depList.Items)

		
		cacheKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		s.analysisCache[cacheKey] = profile

		
		colorHint := "ok"
		if profile.Score < 60 {
			colorHint = "error" 
		} else if profile.Score < 80 {
			colorHint = "warn"  
		}

	
		uiResources = append(uiResources, Resource{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Status:    profile.CriticalityTier, 
			Kind:      "Service",
			Age:       utils.ComputeAge(&svc.CreationTimestamp),

			
			Extras: []string{
				profile.Grade,                    
				string(svc.Spec.Type),            
				svc.Spec.ClusterIP,               
				s.formatPorts(svc.Spec.Ports),    
				fmt.Sprintf("%d", profile.Score), 
				colorHint,                        
			},
		})
	}


	sort.Slice(uiResources, func(i, j int) bool {
		
		scoreI := 100
		scoreJ := 100
		fmt.Sscanf(uiResources[i].Extras[4], "%d", &scoreI)
		fmt.Sscanf(uiResources[j].Extras[4], "%d", &scoreJ)


		return scoreI < scoreJ
	})

	return uiResources, nil
}



func (s *ServiceView) filterPods(allPods []corev1.Pod, selector map[string]string) []corev1.Pod {
	if len(selector) == 0 {
		return nil
	}
	lbls := labels.Set(selector)
	var matches []corev1.Pod
	for _, p := range allPods {
		if lbls.AsSelector().Matches(labels.Set(p.Labels)) {
			matches = append(matches, p)
		}
	}
	return matches
}

func (s *ServiceView) filterIngresses(allIng []networkingv1.Ingress, svcName string) []networkingv1.Ingress {
	var matches []networkingv1.Ingress
	for _, ing := range allIng {
		
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil { continue }
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil && path.Backend.Service.Name == svcName {
					matches = append(matches, ing)
					goto NextIngress 
				}
			}
		}
	NextIngress:
	}
	return matches
}

func (s *ServiceView) filterNetPols(allPol []networkingv1.NetworkPolicy, ns string) []networkingv1.NetworkPolicy {
	var matches []networkingv1.NetworkPolicy
	for _, np := range allPol {
		if np.Namespace == ns {
			matches = append(matches, np)
		}
	}
	return matches
}

func (s *ServiceView) formatPorts(ports []corev1.ServicePort) string {
	var parts []string
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d:%d/%s", p.Port, p.TargetPort.IntVal, p.Protocol))
	}
	return strings.Join(parts, ",")
}