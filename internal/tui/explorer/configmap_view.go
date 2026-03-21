package explorer

import (
	"fmt"
	"sort"
	"time"

	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/core/v1"
)

type ConfigMapView struct {
	lister listers.ConfigMapLister
}

func NewConfigMapView(lister listers.ConfigMapLister) *ConfigMapView {
	return &ConfigMapView{lister: lister}
}

func (c *ConfigMapView) Title() string {
	return "ConfigMaps"
}


func (c *ConfigMapView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
}

func (c *ConfigMapView) Headers() ([]string, []int) {

	return []string{"NAME", "NAMESPACE", "DATA", "AGE"}, []int{70, 25, 10, 10}
}

func (c *ConfigMapView) Retrieve(namespace string) ([]Resource, error) {
	var cms []*corev1.ConfigMap
	var err error

	if namespace == "" {
		cms, err = c.lister.List(labels.Everything())
	} else {
		cms, err = c.lister.ConfigMaps(namespace).List(labels.Everything())
	}

	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, cm := range cms {
		ts := cm.CreationTimestamp.Time
		if cm.CreationTimestamp.IsZero() {
			ts = time.Now()
		}

		uiResources = append(uiResources, Resource{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Status:    fmt.Sprintf("%d", len(cm.Data)), 
			Age:       utils.ComputeAge(&cm.CreationTimestamp),
			Kind:      "ConfigMap",
			
			
			AgeRaw: ts,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}