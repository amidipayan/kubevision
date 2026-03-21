package helm

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type BlastRadius struct {
	Score       int
	Criticals   []ResourceItem 
	Warnings    []ResourceItem 
	Standard    []ResourceItem 
	TotalCount  int
}

type ResourceItem struct {
	Kind string
	Name string
	Info string 
}


func CalculateBlastRadius(objects []*unstructured.Unstructured) BlastRadius {
	report := BlastRadius{}

	for _, obj := range objects {
		item := ResourceItem{
			Kind: obj.GetKind(),
			Name: obj.GetName(),
		}
		report.TotalCount++

		switch item.Kind {
		case "PersistentVolumeClaim":
			item.Info = "DATA LOSS: Storage will be released/deleted"
			report.Criticals = append(report.Criticals, item)
			report.Score += 100

		case "StatefulSet":
			item.Info = "Stateful Workload: Potential data association"
			report.Criticals = append(report.Criticals, item)
			report.Score += 50

		case "Service":
			t, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
			if t == "LoadBalancer" {
				item.Info = "NETWORK LOSS: Public IP will be released"
				report.Criticals = append(report.Criticals, item)
				report.Score += 50
			} else {
				report.Standard = append(report.Standard, item)
			}

		case "Ingress":
			item.Info = "Traffic Route will be removed"
			report.Warnings = append(report.Warnings, item)
			report.Score += 10

		case "Secret":
			t, _, _ := unstructured.NestedString(obj.Object, "type")
			if t != "helm.sh/release.v1" {
				report.Warnings = append(report.Warnings, item)
			} else {
				report.Standard = append(report.Standard, item)
			}

		case "ConfigMap":
			report.Warnings = append(report.Warnings, item)

		case "CustomResourceDefinition":
			item.Info = "GLOBAL: Deleting a CRD can wipe all instances in cluster!"
			report.Criticals = append(report.Criticals, item)
			report.Score += 200

		case "Namespace":
			item.Info = "DESTRUCTION: Entire namespace will be removed"
			report.Criticals = append(report.Criticals, item)
			report.Score += 500

		default:
			report.Standard = append(report.Standard, item)
		}
	}
	return report
}