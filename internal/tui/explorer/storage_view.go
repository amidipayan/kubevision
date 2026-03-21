package explorer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amidipayan/kubevision/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)



type PVCView struct {
	lister corelisters.PersistentVolumeClaimLister
}

func NewPVCView(lister corelisters.PersistentVolumeClaimLister) *PVCView {
	return &PVCView{lister: lister}
}

func (p *PVCView) Title() string { return "PersistentVolumeClaims" }


func (p *PVCView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}
}

func (p *PVCView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "STATUS", "VOLUME", "CAPACITY", "ACCESS", "CLASS", "AGE"}, []int{30, 15, 10, 30, 10, 15, 20, 10}
}

func (p *PVCView) Retrieve(namespace string) ([]Resource, error) {
	var items []*corev1.PersistentVolumeClaim
	var err error
	if namespace == "" {
		items, err = p.lister.List(labels.Everything())
	} else {
		items, err = p.lister.PersistentVolumeClaims(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, pvc := range items {
		cap := pvc.Status.Capacity[corev1.ResourceStorage]
		modes := accessModesToString(pvc.Spec.AccessModes)
		class := ""
		if pvc.Spec.StorageClassName != nil {
			class = *pvc.Spec.StorageClassName
		}

		uiResources = append(uiResources, Resource{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Status:    string(pvc.Status.Phase),
			Kind:      "PersistentVolumeClaim",
			Age:       utils.ComputeAge(&pvc.CreationTimestamp),
			AgeRaw:    pvc.CreationTimestamp.Time,
			Extras:    []string{pvc.Spec.VolumeName, cap.String(), modes, class},
		})
	}
	
	sort.Slice(uiResources, func(i, j int) bool { return uiResources[i].Name < uiResources[j].Name })
	return uiResources, nil
}



type PVView struct {
	lister corelisters.PersistentVolumeLister
}

func NewPVView(lister corelisters.PersistentVolumeLister) *PVView {
	return &PVView{lister: lister}
}

func (p *PVView) Title() string { return "PersistentVolumes" }


func (p *PVView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}
}

func (p *PVView) Headers() ([]string, []int) {
	
	return []string{"NAME", "CAPACITY", "ACCESS", "RECLAIM", "STATUS", "CLAIM", "CLASS", "AGE"}, []int{35, 10, 15, 15, 10, 35, 20, 10}
}

func (p *PVView) Retrieve(namespace string) ([]Resource, error) {
	
	items, err := p.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, pv := range items {
		cap := pv.Spec.Capacity[corev1.ResourceStorage]
		modes := accessModesToString(pv.Spec.AccessModes)
		claimRef := ""
		if pv.Spec.ClaimRef != nil {
			claimRef = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}

		uiResources = append(uiResources, Resource{
			Name:      pv.Name,
			Namespace: "", 
			Status:    string(pv.Status.Phase),
			Kind:      "PersistentVolume",
			Age:       utils.ComputeAge(&pv.CreationTimestamp),
			AgeRaw:    pv.CreationTimestamp.Time,
			Extras:    []string{cap.String(), modes, string(pv.Spec.PersistentVolumeReclaimPolicy), claimRef, pv.Spec.StorageClassName},
		})
	}
	sort.Slice(uiResources, func(i, j int) bool { return uiResources[i].Name < uiResources[j].Name })
	return uiResources, nil
}



type SCView struct {
	lister storagelisters.StorageClassLister
}

func NewSCView(lister storagelisters.StorageClassLister) *SCView {
	return &SCView{lister: lister}
}

func (s *SCView) Title() string { return "StorageClasses" }


func (s *SCView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
}

func (s *SCView) Headers() ([]string, []int) {
	
	return []string{"NAME", "PROVISIONER", "RECLAIM POLICY", "AGE"}, []int{35, 40, 20, 10}
}

func (s *SCView) Retrieve(namespace string) ([]Resource, error) {
	items, err := s.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, sc := range items {
		reclaim := "<nil>"
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}

		uiResources = append(uiResources, Resource{
			Name:      sc.Name,
			Kind:      "StorageClass",
			Age:       utils.ComputeAge(&sc.CreationTimestamp),
			AgeRaw:    sc.CreationTimestamp.Time,
			Extras:    []string{sc.Provisioner, reclaim},
		})
	}
	sort.Slice(uiResources, func(i, j int) bool { return uiResources[i].Name < uiResources[j].Name })
	return uiResources, nil
}


func accessModesToString(modes []corev1.PersistentVolumeAccessMode) string {
	var s []string
	for _, m := range modes {
		switch m {
		case corev1.ReadWriteOnce:
			s = append(s, "RWO")
		case corev1.ReadOnlyMany:
			s = append(s, "ROX")
		case corev1.ReadWriteMany:
			s = append(s, "RWX")
		case corev1.ReadWriteOncePod:
			s = append(s, "RWOP")
		}
	}
	return strings.Join(s, ",")
}