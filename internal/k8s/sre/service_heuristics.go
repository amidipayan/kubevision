package sre

import (
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes"
)



type RiskLevel string

const (
	RiskNone     RiskLevel = "NONE"
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
)


type SREProfile struct {
	
	CriticalityScore int    
	CriticalityTier  string 

	
	RRI            float64 
	ExposureScore  float64 
	BlindnessScore float64 
	ChangeRisk     float64 

	
	Score        int     
	Grade        string   
	HumanVerdict string   
	RiskFactors  []string 
}


func AnalyzeService(
	clientset kubernetes.Interface,
	svc corev1.Service,
	pods []corev1.Pod,
	ingresses []networkingv1.Ingress,
	netpols []networkingv1.NetworkPolicy,
	deployments []appsv1.Deployment,
) SREProfile {

	
	critScore, critTier := calculateCriticality(svc, ingresses)

	
	rri, rriReason := calculateRRI(critScore, pods, svc, netpols)
	exposure, exposureReason := calculateExposure(svc, netpols, ingresses)
	blindness, blindReason := calculateBlindness(pods)
	changeRisk, changeReason := calculateChangeSafety(pods, deployments, svc.Name)

	
	ageRisk := calculateAgingRisk(pods, exposure)

	
	totalPenalty := 0.0

	
	wReliability := 40.0
	wSecurity := 30.0
	wObs := 15.0
	wChange := 15.0

	
	if exposure > 0.7 {
		wSecurity = 50.0
		wReliability = 20.0
	}

	totalPenalty += rri * wReliability
	totalPenalty += exposure * wSecurity
	totalPenalty += blindness * wObs
	totalPenalty += changeRisk * wChange
	totalPenalty += ageRisk * 5.0 

	finalScore := int(100 - totalPenalty)
	if finalScore < 0 {
		finalScore = 0
	}

	
	verdict := rriReason
	if exposure > rri {
		verdict = exposureReason
	}
	if changeRisk > 0.8 {
		verdict = changeReason 
	}

	
	var factors []string
	if rri > 0.3 {
		factors = append(factors, fmt.Sprintf("Reliability Risk: %s", rriReason))
	}
	if exposure > 0.3 {
		factors = append(factors, fmt.Sprintf("Security: %s", exposureReason))
	}
	if blindness > 0.3 {
		factors = append(factors, fmt.Sprintf("Blindness: %s", blindReason))
	}
	if changeRisk > 0.3 {
		factors = append(factors, fmt.Sprintf("Deployment: %s", changeReason))
	}

	return SREProfile{
		CriticalityScore: critScore,
		CriticalityTier:  critTier,
		RRI:              rri,
		ExposureScore:    exposure,
		BlindnessScore:   blindness,
		ChangeRisk:       changeRisk,
		Score:            finalScore,
		Grade:            calculateGrade(finalScore),
		HumanVerdict:     verdict,
		RiskFactors:      factors,
	}
}


func calculateCriticality(svc corev1.Service, ingresses []networkingv1.Ingress) (int, string) {
	score := 0

	
	isInternet := false
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || len(ingresses) > 0 {
		score += 30
		isInternet = true
	}

	
	ns := strings.ToLower(svc.Namespace)
	if ns == "kube-system" || ns == "ingress-nginx" || ns == "istio-system" || ns == "auth" {
		score += 25
	} else if ns == "default" || ns == "prod" {
		score += 10
	}

	
	name := strings.ToLower(svc.Name)
	if strings.Contains(name, "db") || strings.Contains(name, "data") || strings.Contains(name, "redis") || strings.Contains(name, "auth") {
		score += 15
	}

	
	if svc.Spec.Type == corev1.ServiceTypeClusterIP && !isInternet {
		
		score += 5
	}

	
	if score > 100 {
		score = 100
	}

	
	tier := "Tier 4 (Internal)"
	if score >= 80 {
		tier = "Tier 1 (Critical)"
	} else if score >= 50 {
		tier = "Tier 2 (Important)"
	} else if score >= 30 {
		tier = "Tier 3 (Supporting)"
	}

	return score, tier
}


func calculateRRI(critScore int, pods []corev1.Pod, svc corev1.Service, netpols []networkingv1.NetworkPolicy) (float64, string) {
	if len(pods) == 0 {
		return 1.0, "Service has no active endpoints (Outage)"
	}

	failureProb := 0.0
	reason := "Stable"

	
	if len(pods) == 1 {
		failureProb += 0.4
		reason = "Single point of failure (1 replica)"
	}

	
	if len(pods) > 1 {
		hasAntiAffinity := false
		if pods[0].Spec.Affinity != nil && pods[0].Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAffinity = true
		}
		if !hasAntiAffinity {
			failureProb += 0.2
			if reason == "Stable" {
				reason = "No Anti-Affinity (Node drain risk)"
			}
		}
	}

	
	if pods[0].Status.QOSClass == corev1.PodQOSBestEffort {
		failureProb += 0.3
		if reason == "Stable" {
			reason = "BestEffort QoS (OOMKill candidate)"
		}
	}

	
	if critScore > 70 {
		failureProb *= 1.5
	}

	if failureProb > 1.0 {
		failureProb = 1.0
	}
	return failureProb, reason
}


func calculateExposure(svc corev1.Service, netpols []networkingv1.NetworkPolicy, ingresses []networkingv1.Ingress) (float64, string) {
	exposure := 0.0
	reason := "Internal/Protected"

	
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || len(ingresses) > 0 {
		exposure += 0.6
		reason = "Exposed to Internet"
	} else if svc.Spec.Type == corev1.ServiceTypeNodePort {
		exposure += 0.4
		reason = "NodePort Exposure (Cluster-wide)"
	} else {
		
		exposure += 0.1
	}

	
	if len(netpols) == 0 {
		exposure += 0.4
		if reason == "Internal/Protected" {
			reason = "Zero Trust violation (No NetPol)"
		}
	}

	if exposure > 1.0 {
		exposure = 1.0
	}
	return exposure, reason
}


func calculateBlindness(pods []corev1.Pod) (float64, string) {
	if len(pods) == 0 {
		return 1.0, "No Pods"
	}

	blindness := 0.0
	reason := "Observable"

	
	if pods[0].Spec.Containers[0].LivenessProbe == nil {
		blindness += 0.4
		reason = "Missing Liveness Probe"
	}

	
	if pods[0].Spec.Containers[0].ReadinessProbe == nil {
		blindness += 0.4
		if reason == "Observable" {
			reason = "Missing Readiness Probe"
		}
	}

	return blindness, reason
}


func calculateChangeSafety(pods []corev1.Pod, deployments []appsv1.Deployment, svcName string) (float64, string) {
	
	var targetDep *appsv1.Deployment
	for _, d := range deployments {
		
		if strings.HasPrefix(d.Name, svcName) || strings.HasPrefix(svcName, d.Name) {
			targetDep = &d
			break
		}
	}

	if targetDep == nil {
		return 0.0, "No Deployment Found"
	}

	risk := 0.0
	reason := "Safe Deployment Strategy"

	
	if targetDep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
		risk += 0.8
		reason = "Recreate Strategy (Guaranteed Downtime)"
	} else if targetDep.Spec.Strategy.RollingUpdate != nil {
		
		maxUnavail := targetDep.Spec.Strategy.RollingUpdate.MaxUnavailable
		if maxUnavail != nil && (maxUnavail.StrVal == "100%" || maxUnavail.IntVal == int32(*targetDep.Spec.Replicas)) {
			risk += 0.6
			reason = "Aggressive RollingUpdate (High MaxUnavailable)"
		}
	}

	
	if len(pods) > 0 {
		image := pods[0].Spec.Containers[0].Image
		if strings.HasSuffix(image, ":latest") {
			risk += 0.5
			if reason == "Safe Deployment Strategy" {
				reason = "Mutable Tag (:latest)"
			}
		}
	}

	if risk > 1.0 {
		risk = 1.0
	}
	return risk, reason
}


func calculateAgingRisk(pods []corev1.Pod, exposure float64) float64 {
	if len(pods) == 0 {
		return 0.0
	}

	created := pods[0].CreationTimestamp.Time
	ageHours := time.Since(created).Hours()

	if ageHours > (24 * 90) { 
		if exposure > 0.5 {
			return 0.3 
		}
		return 0.1 
	}

	return 0.0
}

func calculateGrade(score int) string {
	if score >= 90 {
		return "A+"
	}
	if score >= 85 {
		return "A"
	}
	if score >= 75 {
		return "B"
	}
	if score >= 60 {
		return "C"
	}
	if score >= 40 {
		return "D"
	}
	return "F"
}