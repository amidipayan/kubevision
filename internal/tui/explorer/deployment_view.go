package explorer

import (
	"fmt"
	"sort"
	"time"

	"github.com/amidipayan/kubevision/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/apps/v1"
)

type DeploymentView struct {
	lister listers.DeploymentLister
}

func NewDeploymentView(lister listers.DeploymentLister) *DeploymentView {
	return &DeploymentView{lister: lister}
}

func (d *DeploymentView) Title() string {
	return "Deployments"
}


func (d *DeploymentView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
}

func (d *DeploymentView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "READY", "AGE"}, []int{45, 15, 15, 10}
}

func (d *DeploymentView) Retrieve(namespace string) ([]Resource, error) {
	var deps []*appsv1.Deployment
	var err error

	if namespace == "" {
		deps, err = d.lister.List(labels.Everything())
	} else {
		deps, err = d.lister.Deployments(namespace).List(labels.Everything())
	}

	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, dep := range deps {
		// Handle nil timestamp safely
		ts := dep.CreationTimestamp.Time
		if dep.CreationTimestamp.IsZero() {
			ts = time.Now()
		}

		uiResources = append(uiResources, Resource{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Status:    fmt.Sprintf("%d/%d", dep.Status.ReadyReplicas, dep.Status.Replicas),
			Age:       utils.ComputeAge(&dep.CreationTimestamp),
			Kind:      "Deployment",
			
			// Populate Raw Sort Field
			AgeRaw: ts,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}