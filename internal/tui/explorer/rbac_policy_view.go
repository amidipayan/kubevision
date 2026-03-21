package explorer

import (
	"github.com/amidipayan/kubevision/internal/k8s/auth"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
)

type RBACPolicyView struct {
	scanner  *auth.RBACScanner
	subjects []auth.SubjectRef
	title    string
}

func NewRBACPolicyView(factory informers.SharedInformerFactory, subjects []auth.SubjectRef, titleOverride string) *RBACPolicyView {
	return &RBACPolicyView{
		scanner:  auth.NewRBACScanner(factory),
		subjects: subjects,
		title:    titleOverride,
	}
}

func (r *RBACPolicyView) Title() string {
	if r.title != "" {
		return r.title
	}
	return "RBAC Policy"
}


func (r *RBACPolicyView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}


func (r *RBACPolicyView) Headers() ([]string, []int) {
	return []string{
		"NAME", "APIGROUP",
		"GET", "LIST", "WATCH", "CREATE", "PATCH", "UPDATE", "DELETE", "DEL-LIST",
	}, []int{
		30, 25, 
		6, 6, 8, 8, 8, 8, 8, 10, 
	}
}

func (r *RBACPolicyView) Retrieve(namespace string) ([]Resource, error) {
	rows, err := r.scanner.FetchAggregatedRules(r.subjects)
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, row := range rows {
		checks := []string{
			renderMarker(row.Get),
			renderMarker(row.List),
			renderMarker(row.Watch),
			renderMarker(row.Create),
			renderMarker(row.Patch),
			renderMarker(row.Update),
			renderMarker(row.Delete),
			renderMarker(row.DelList),
		}

		grp := row.APIGroup
		if grp == "" {
			grp = "-" 
		}

		uiResources = append(uiResources, Resource{
			Name:   row.Resource,
			Kind:   grp,
			Extras: checks,
		})
	}
	return uiResources, nil
}

func renderMarker(allowed bool) string {
	if allowed {
		return "✓"
	}
	return ""
}