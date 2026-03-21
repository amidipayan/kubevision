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

type ReplicaSetView struct {
	lister listers.ReplicaSetLister
}

func NewReplicaSetView(lister listers.ReplicaSetLister) *ReplicaSetView {
	return &ReplicaSetView{lister: lister}
}

func (r *ReplicaSetView) Title() string {
	return "ReplicaSets"
}


func (r *ReplicaSetView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
}

func (r *ReplicaSetView) Headers() ([]string, []int) {

	return []string{"NAME", "NAMESPACE", "DESIRED/CUR/READY", "AGE"}, []int{45, 15, 20, 10}
}

func (r *ReplicaSetView) Retrieve(namespace string) ([]Resource, error) {
	var rss []*appsv1.ReplicaSet
	var err error

	if namespace == "" {
		rss, err = r.lister.List(labels.Everything())
	} else {
		rss, err = r.lister.ReplicaSets(namespace).List(labels.Everything())
	}

	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, rs := range rss {
		ts := rs.CreationTimestamp.Time
		if rs.CreationTimestamp.IsZero() {
			ts = time.Now()
		}

		status := fmt.Sprintf("%d / %d / %d", *rs.Spec.Replicas, rs.Status.Replicas, rs.Status.ReadyReplicas)
		uiResources = append(uiResources, Resource{
			Name:      rs.Name,
			Namespace: rs.Namespace,
			Status:    status,
			Age:       utils.ComputeAge(&rs.CreationTimestamp),
			Kind:      "ReplicaSet",

			
			AgeRaw: ts,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}