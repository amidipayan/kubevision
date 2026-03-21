package explorer

import (
	"sort"

	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/core/v1"
)

type SecretView struct {
	lister listers.SecretLister
}

func NewSecretView(lister listers.SecretLister) *SecretView {
	return &SecretView{lister: lister}
}

func (s *SecretView) Title() string {
	return "Secrets"
}


func (s *SecretView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
}

func (s *SecretView) Headers() ([]string, []int) {

	return []string{"NAME", "NAMESPACE", "TYPE", "AGE"}, []int{45, 15, 40, 10}
}

func (s *SecretView) Retrieve(namespace string) ([]Resource, error) {
	var secrets []*corev1.Secret
	var err error

	if namespace == "" {
		secrets, err = s.lister.List(labels.Everything())
	} else {
		secrets, err = s.lister.Secrets(namespace).List(labels.Everything())
	}

	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, sec := range secrets {
		uiResources = append(uiResources, Resource{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Status:    string(sec.Type),
			Age:       utils.ComputeAge(&sec.CreationTimestamp),
			Kind:      "Secret",
			
			AgeRaw: sec.CreationTimestamp.Time,
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}