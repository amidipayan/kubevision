package helm

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)


const (
	RiskHigh   = "HIGH"
	RiskMedium = "MEDIUM"
	RiskLow    = "LOW"
)

type RiskFactor struct {
	Level   string
	Icon    string
	Title   string
	Message string
}


type RiskAssessment struct {
	Score       int 
	Factors     []RiskFactor
	HasCritical bool
}


func AnalyzeUpgradeRisks(oldObjs, newObjs []*unstructured.Unstructured) RiskAssessment {
	assessment := RiskAssessment{
		Factors: []RiskFactor{},
	}

	
	oldMap := make(map[string]*unstructured.Unstructured)
	for _, o := range oldObjs {
		key := fmt.Sprintf("%s/%s/%s", o.GetKind(), o.GetNamespace(), o.GetName())
		oldMap[key] = o
	}

	newMap := make(map[string]*unstructured.Unstructured)
	for _, o := range newObjs {
		key := fmt.Sprintf("%s/%s/%s", o.GetKind(), o.GetNamespace(), o.GetName())
		newMap[key] = o
	}

	
	for key, oldObj := range oldMap {
		if _, exists := newMap[key]; !exists {
			
			if oldObj.GetKind() == "PersistentVolumeClaim" {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskHigh,
					Icon:    "🗑️",
					Title:   "PVC Deletion",
					Message: fmt.Sprintf("PersistentVolumeClaim '%s' will be removed. DATA LOSS POSSIBLE.", oldObj.GetName()),
				})
				assessment.HasCritical = true
				assessment.Score += 50
			} else if oldObj.GetKind() == "Service" {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskMedium,
					Icon:    "🔌",
					Title:   "Service Removal",
					Message: fmt.Sprintf("Service '%s' is being removed. Connectivity may break.", oldObj.GetName()),
				})
				assessment.Score += 20
			} else if oldObj.GetKind() == "StatefulSet" {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskHigh,
					Icon:    "⚠️",
					Title:   "StatefulSet Removal",
					Message: fmt.Sprintf("StatefulSet '%s' is being removed. Check for orphaned pods/PVCs.", oldObj.GetName()),
				})
				assessment.Score += 30
			}
		}
	}

	
	for key, newObj := range newMap {
		oldObj, exists := oldMap[key]

		
		if exists {
			checkOperationalRisks(newObj, oldObj, &assessment)
		}


		checkSecurityRisks(newObj, oldObj, &assessment)
	}

	return assessment
}

func checkOperationalRisks(newObj, oldObj *unstructured.Unstructured, assessment *RiskAssessment) {
	switch newObj.GetKind() {
	case "Service":
		oldType, _, _ := unstructured.NestedString(oldObj.Object, "spec", "type")
		newType, _, _ := unstructured.NestedString(newObj.Object, "spec", "type")
		if oldType == "LoadBalancer" && newType != "LoadBalancer" && newType != "" {
			assessment.Factors = append(assessment.Factors, RiskFactor{
				Level:   RiskHigh,
				Icon:    "📉",
				Title:   "Service Downgrade",
				Message: fmt.Sprintf("Service '%s' changed from %s to %s. External access lost.", newObj.GetName(), oldType, newType),
			})
			assessment.Score += 40
		}

	case "Deployment", "StatefulSet":
		oldReplicas, foundOld, _ := unstructured.NestedInt64(oldObj.Object, "spec", "replicas")
		newReplicas, foundNew, _ := unstructured.NestedInt64(newObj.Object, "spec", "replicas")
		if foundOld && foundNew && newReplicas < oldReplicas {
			drop := float64(oldReplicas-newReplicas) / float64(oldReplicas)
			if drop >= 0.5 {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskHigh,
					Icon:    "📉",
					Title:   "Drastic Scale Down",
					Message: fmt.Sprintf("%s '%s' replicas reduced by %.0f%% (%d -> %d).", newObj.GetKind(), newObj.GetName(), drop*100, oldReplicas, newReplicas),
				})
				assessment.Score += 30
			} else {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskLow,
					Icon:    "📉",
					Title:   "Scale Down",
					Message: fmt.Sprintf("%s '%s' replicas reduced (%d -> %d).", newObj.GetKind(), newObj.GetName(), oldReplicas, newReplicas),
				})
				assessment.Score += 5
			}
		}
		
		
		checkImageChanges(oldObj, newObj, assessment)
	}
}


func checkSecurityRisks(newObj, oldObj *unstructured.Unstructured, assessment *RiskAssessment) {
	kind := newObj.GetKind()
	if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" && kind != "Pod" {
		return
	}

	
	getPodSpec := func(obj *unstructured.Unstructured) (map[string]interface{}, bool) {
		if obj == nil { return nil, false }
		var spec map[string]interface{}
		var found bool
		
		if obj.GetKind() == "Pod" {
			spec, found, _ = unstructured.NestedMap(obj.Object, "spec")
		} else {
			spec, found, _ = unstructured.NestedMap(obj.Object, "spec", "template", "spec")
		}
		return spec, found
	}

	newSpec, found := getPodSpec(newObj)
	if !found { return }
	
	oldSpec, _ := getPodSpec(oldObj) 

	
	checkHostNetwork(newObj.GetName(), newSpec, oldSpec, assessment)

	
	checkContainersSecurity(newObj.GetName(), newSpec, oldSpec, assessment)
}

func checkHostNetwork(name string, newSpec, oldSpec map[string]interface{}, assessment *RiskAssessment) {
	newHost, _, _ := unstructured.NestedBool(newSpec, "hostNetwork")
	oldHost, _, _ := unstructured.NestedBool(oldSpec, "hostNetwork")

	
	if newHost && !oldHost {
		assessment.Factors = append(assessment.Factors, RiskFactor{
			Level:   RiskHigh,
			Icon:    "🔓",
			Title:   "Host Network Exposed",
			Message: fmt.Sprintf("Workload '%s' now uses hostNetwork. This bypasses network isolation.", name),
		})
		assessment.Score += 25
		assessment.HasCritical = true
	}
}

func checkContainersSecurity(resName string, newSpec, oldSpec map[string]interface{}, assessment *RiskAssessment) {
	
	oldContainers := make(map[string]map[string]interface{})
	if oldSpec != nil {
		ctrs, _, _ := unstructured.NestedSlice(oldSpec, "containers")
		for _, c := range ctrs {
			if cmap, ok := c.(map[string]interface{}); ok {
				if name, _, _ := unstructured.NestedString(cmap, "name"); name != "" {
					oldContainers[name] = cmap
				}
			}
		}
	}

	newContainers, _, _ := unstructured.NestedSlice(newSpec, "containers")
	for _, c := range newContainers {
		cmap, ok := c.(map[string]interface{})
		if !ok { continue }
		
		cName, _, _ := unstructured.NestedString(cmap, "name")
		oldCmap := oldContainers[cName] 

		
		newPriv, _, _ := unstructured.NestedBool(cmap, "securityContext", "privileged")
		oldPriv, _, _ := unstructured.NestedBool(oldCmap, "securityContext", "privileged")

		if newPriv && !oldPriv {
			assessment.Factors = append(assessment.Factors, RiskFactor{
				Level:   RiskHigh,
				Icon:    "🔥",
				Title:   "Privileged Container",
				Message: fmt.Sprintf("Container '%s' in '%s' is now PRIVILEGED. Root access to node.", cName, resName),
			})
			assessment.Score += 40
			assessment.HasCritical = true
		}


		newUid, newSet, _ := unstructured.NestedInt64(cmap, "securityContext", "runAsUser")
		
		if newSet && newUid == 0 {
			
			oldUid, oldSet, _ := unstructured.NestedInt64(oldCmap, "securityContext", "runAsUser")
			if !oldSet || oldUid != 0 {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskMedium,
					Icon:    "👑",
					Title:   "Running as Root",
					Message: fmt.Sprintf("Container '%s' explicitly set to run as UID 0 (Root).", cName),
				})
				assessment.Score += 10
			}
		}
	}
}

func checkImageChanges(oldObj, newObj *unstructured.Unstructured, assessment *RiskAssessment) {
	
	containers, found, _ := unstructured.NestedSlice(newObj.Object, "spec", "template", "spec", "containers")
	if !found {
		return
	}
	
	
	getImages := func(obj *unstructured.Unstructured) map[string]string {
		imgs := make(map[string]string)
		ctrs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		for _, c := range ctrs {
			cmap, _ := c.(map[string]interface{})
			name, _ := cmap["name"].(string)
			img, _ := cmap["image"].(string)
			imgs[name] = img
		}
		return imgs
	}

	oldImages := getImages(oldObj)
	
	for _, c := range containers {
		cmap, _ := c.(map[string]interface{})
		name, _ := cmap["name"].(string)
		newImg, _ := cmap["image"].(string)
		
		if oldImg, ok := oldImages[name]; ok && oldImg != newImg {
			if strings.HasSuffix(newImg, ":latest") {
				assessment.Factors = append(assessment.Factors, RiskFactor{
					Level:   RiskMedium,
					Icon:    "🏷️",
					Title:   "Using 'latest' Tag",
					Message: fmt.Sprintf("Container '%s' updated to '%s'. Avoid 'latest' in production.", name, newImg),
				})
				assessment.Score += 10
			}
		}
	}
}