package explorer

import (
	"fmt"
	"time"

	"github.com/amidipayan/kubevision/internal/k8s/auth"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
)

type RBACView struct {
	scanner *auth.RBACScanner
}

func NewRBACView(factory informers.SharedInformerFactory) *RBACView {
	return &RBACView{
		scanner: auth.NewRBACScanner(factory),
	}
}

func (r *RBACView) Title() string {
	return "RBAC Subjects"
}


func (r *RBACView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}

func (r *RBACView) Headers() ([]string, []int) {
	
	return []string{"SUBJECT NAME", "KIND", "NAMESPACE", "BINDINGS (CR/R)", "ACCESS LEVEL"}, []int{40, 15, 20, 15, 20}
}

func (r *RBACView) Retrieve(namespace string) ([]Resource, error) {
	summaries, err := r.scanner.ListSubjects()
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, s := range summaries {
	
		if namespace != "" && s.Subject.Namespace != "" && s.Subject.Namespace != namespace {
			continue
		}

		
		accessLevel := "Standard"
		if s.IsAdmin {
			accessLevel = "🔥 CLUSTER ADMIN"
		} else if s.ClusterRoleCount > 0 {
			accessLevel = "Cluster Scope"
		}

		uiResources = append(uiResources, Resource{
			Name:      s.Subject.Name,
			Namespace: s.Subject.Namespace, 
			Kind:      s.Subject.Kind,
			Status:    accessLevel, 
			
			
			CPU:       fmt.Sprintf("%d / %d", s.ClusterRoleCount, s.RoleCount),
			
			
			Age:       "-", 
			Memory:    "-",

			
			AgeRaw:    time.Now(), 
		})
	}
	return uiResources, nil
}