package helm

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)


func AnalyzeSecurityPosture(rel *release.Release, objs []*unstructured.Unstructured, history []*release.Release) SecurityReport {
	
	report := SecurityReport{
		ScoreRBAC:      100,
		ScoreHardening: 100,
		ScoreImages:    100,
		ScoreSecrets:   100,
		ScoreExposure:  100,
		ScoreSupply:    100,
		EvaluatedAt:    time.Now(),
		Explainability: make(map[string][]CheckResult),
		NextSteps:      []string{},
		RiskVelocity:   "Stable",
		Findings:       []CheckResult{}, 
	}


	record := func(category string, pass bool, severity, confidence, effort, msg, why, fix, code string, impact int) {
		report.ChecksTotal++
		if pass {
			report.ChecksPassed++
		} else {
			report.ChecksFailed++
			
			
			report.Findings = append(report.Findings, CheckResult{
				Passed:          false,
				Category:        category,
				Severity:        severity,
				Confidence:      confidence,
				Effort:          effort,
				Message:         msg,
				Why:             why,
				Remediation:     fix,
				RemediationCode: code,
				ScoreImpact:     impact,
			})
		}
		
		
		report.Explainability[category] = append(report.Explainability[category], CheckResult{
			Passed:          pass,
			Severity:        severity,
			Confidence:      confidence,
			Effort:          effort,
			Message:         msg,
			Why:             why,
			Remediation:     fix,
			RemediationCode: code,
			ScoreImpact:     impact,
		})
	}

	
	blast := BlastRadiusContext{
		NamespaceLevel: "Isolated",
		NetworkLevel:   "Locked",
		RBACLevel:      "None",
		PrivilegeLevel: "Restricted",
		Score:          1.0, 
	}

	
	hasNetworkPolicy := false
	for _, obj := range objs {
		if obj.GetKind() == "NetworkPolicy" {
			hasNetworkPolicy = true
		}
	}

	
	for _, obj := range objs {
		kind := obj.GetKind()
		name := obj.GetName()

		
		annotations, found, _ := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
		if found {
			if hook, ok := annotations["helm.sh/hook"]; ok {
				record("Hardening", false, "LOW", "High", "MED", fmt.Sprintf("Resource '%s' is a Helm Hook (%s)", name, hook),
					"Hooks run outside standard lifecycle management and can bypass policy constraints.",
					"Audit hook permissions and ensure it's strictly necessary.", "", 5)
			}
		}

		
		if kind == "ClusterRole" || kind == "ClusterRoleBinding" {
			blast.RBACLevel = "Cluster-Admin"
			blast.Score += 2.0
			report.ScoreRBAC -= 20
			record("RBAC", false, "HIGH", "High", "MED", "Installs ClusterRole (Global Access)",
				"ClusterRoles bypass namespace isolation, increasing blast radius to the entire cluster.",
				"Use Roles and RoleBindings instead of ClusterRoles where possible.",
				"kind: Role\nmetadata:\n  namespace: "+rel.Namespace, 20)
		}

		if kind == "ServiceAccount" {
			automount, found, _ := unstructured.NestedBool(obj.Object, "automountServiceAccountToken")
			if found && automount {
				report.ScoreRBAC -= 5
				record("RBAC", false, "MED", "High", "EASY", "ServiceAccount automounts API token",
					"Mounting API tokens allows compromised pods to query the K8s API.",
					"Set 'automountServiceAccountToken: false' in ServiceAccount.",
					"automountServiceAccountToken: false", 5)
			}
		}

		if kind == "Role" || kind == "ClusterRole" {
			rules, found, _ := unstructured.NestedSlice(obj.Object, "rules")
			hasWildcard := false
			if found {
				for _, r := range rules {
					rule := r.(map[string]interface{})
					verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
					for _, v := range verbs {
						if v == "*" {
							hasWildcard = true
						}
					}
				}
			}
			if hasWildcard {
				report.ScoreRBAC -= 10
				record("RBAC", false, "HIGH", "High", "MED", fmt.Sprintf("%s uses wildcard (*) verbs", name),
					"Wildcard permissions violate the principle of Least Privilege.",
					"Explicitly list required verbs (e.g., 'get', 'list', 'watch').", "", 10)
			}
		}

		
		if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Job" {
			containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			if found {
				for _, c := range containers {
					cmap, _ := c.(map[string]interface{})
					
					
					if priv, _, _ := unstructured.NestedBool(cmap, "securityContext", "privileged"); priv {
						blast.PrivilegeLevel = "Root"
						blast.Score += 1.0
						report.ScoreHardening -= 30
						record("Hardening", false, "CRITICAL", "High", "EASY", "Container runs as Privileged",
							"Privileged containers have host root capabilities and can escape isolation.",
							"Remove 'securityContext.privileged: true'.",
							"securityContext:\n  privileged: false", 30)
					} else {
						record("Hardening", true, "INFO", "High", "", "Container runs unprivileged", "", "", "", 0)
					}

					
					if ape, foundAPE, _ := unstructured.NestedBool(cmap, "securityContext", "allowPrivilegeEscalation"); !foundAPE || ape {
						report.ScoreHardening -= 10
						record("Hardening", false, "MED", "Med", "EASY", "Privilege Escalation allowed",
							"Child processes can gain more privileges than the parent (e.g. sudo).",
							"Set 'securityContext.allowPrivilegeEscalation: false'.",
							"securityContext:\n  allowPrivilegeEscalation: false", 10)
					}

					
					caps, foundCaps, _ := unstructured.NestedSlice(cmap, "securityContext", "capabilities", "drop")
					hasDropAll := false
					if foundCaps {
						for _, cap := range caps {
							if capStr, ok := cap.(string); ok && (capStr == "ALL" || capStr == "-ALL") {
								hasDropAll = true
							}
						}
					}
					if !hasDropAll {
						report.ScoreHardening -= 5
						record("Hardening", false, "LOW", "Low", "HARD", "Capabilities not dropped (ALL)",
							"Default capabilities include potentially dangerous syscalls.",
							"Set 'securityContext.capabilities.drop: [\"ALL\"]'.",
							"securityContext:\n  capabilities:\n    drop: [\"ALL\"]", 5)
					}

					
					if ro, _, _ := unstructured.NestedBool(cmap, "securityContext", "readOnlyRootFilesystem"); !ro {
						report.ScoreHardening -= 5
						record("Hardening", false, "MED", "Med", "MED", "Root filesystem is writable",
							"Writable root allows attackers to install tools or modify configuration.",
							"Set 'securityContext.readOnlyRootFilesystem: true'.",
							"securityContext:\n  readOnlyRootFilesystem: true", 5)
					}

					
					image, _, _ := unstructured.NestedString(cmap, "image")
					if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
						report.ScoreImages -= 20
						record("Images", false, "MED", "Med", "EASY", "Uses mutable 'latest' tag",
							"Tags like 'latest' can change, breaking immutability and rollback.",
							"Pin image to a specific version or SHA256 digest.",
							"image: repo/app:v1.2.3", 20)
						
						
						exists := false
						for _, s := range report.NextSteps {
							if strings.Contains(s, "Pin images") {
								exists = true
							}
						}
						if !exists {
							report.NextSteps = append(report.NextSteps, "Pin images to SHA256 digest or immutable version tags")
						}
					}

					
					if !strings.Contains(image, ".") && !strings.Contains(image, "localhost") {
						
						record("Images", false, "LOW", "Low", "MED", "Image from public Docker Hub",
							"Public images may have rate limits or unverified supply chains.",
							"Mirror images to a private registry.", "", 5)
					}
				}
			}

			
			if hn, _, _ := unstructured.NestedBool(obj.Object, "spec", "template", "spec", "hostNetwork"); hn {
				blast.NetworkLevel = "Open East-West"
				blast.Score += 1.0
				report.ScoreExposure -= 25
				record("Exposure", false, "HIGH", "High", "MED", "Workload uses Host Network",
					"HostNetwork allows traffic interception and bypasses network policies.",
					"Remove 'hostNetwork: true' unless strictly required.",
					"hostNetwork: false", 25)
			}

			
			if !hasNetworkPolicy {
				report.ScoreExposure -= 5
				record("Exposure", false, "LOW", "Low", "HARD", "No NetworkPolicy defined for workload",
					"Without NetworkPolicies, all pod-to-pod traffic is allowed.",
					"Define a NetworkPolicy to whitelist traffic.",
					"kind: NetworkPolicy\nspec:\n  podSelector: {}", 5)
			}
		}

		
		if kind == "Service" {
			sType, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
			if sType == "LoadBalancer" || sType == "NodePort" {
				blast.NetworkLevel = "Public"
				blast.Score += 0.5
				report.ScoreExposure -= 5
				record("Exposure", false, "LOW", "High", "HARD", fmt.Sprintf("Service %s is exposed (%s)", name, sType),
					"Public services increase attack surface.",
					"Ensure firewall rules or Ingress controllers are properly secured.", "", 5)
			}
		}
	}

	
	if rel != nil && rel.Chart != nil && rel.Chart.Metadata != nil {
		md := rel.Chart.Metadata
		if md.Home == "" && len(md.Sources) == 0 {
			report.ScoreSupply -= 10
			record("Supply Chain", false, "LOW", "Low", "EASY", "Chart missing provenance metadata",
				"No Home or Source URL provided in Chart.yaml.",
				"Update Chart.yaml with upstream source URLs.", "", 10)
		}
		
		
		for _, dep := range md.Dependencies {
			if dep.Condition != "" {
				report.ScoreSupply -= 5
				record("Supply Chain", false, "MED", "Med", "MED", fmt.Sprintf("Dependency '%s' is conditional", dep.Name),
					"Conditional dependencies can lead to unpredictable deployments.",
					"Use explicit tags or separate releases instead of toggles.", "", 5)
			}
		}
	}

	
	if rel != nil {
		
		valMap := rel.Config
		
		
		if fmt.Sprintf("%v", valMap) != "" {
			if len(fmt.Sprintf("%v", valMap)) > 5000 {
				record("Configuration", false, "LOW", "Med", "HARD", "Large Values Override detected",
					"Heavy reliance on values.yaml overrides makes charts hard to upgrade.",
					"Fork the chart or contribute upstream changes.", "", 5)
			}
		}

		flattened := flattenValues(valMap, "")
		for k, v := range flattened {
			key := strings.ToLower(k)

			
			if strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") {
				if strVal, ok := v.(string); ok && len(strVal) > 0 && strVal != "nil" && !strings.HasPrefix(strVal, "ExistingSecret") {
					report.ScoreSecrets -= 10
					record("Secrets", false, "HIGH", "High", "MED", fmt.Sprintf("Potential plaintext secret: %s", k),
						"Storing secrets in values.yaml commits them to Git in plaintext.",
						"Use ExternalSecrets, SealedSecrets, or 'existingSecret' references.", "", 10)
				}
			}

			
			if strings.Contains(key, "privileged") && v == true {
				report.ScoreHardening -= 10
				record("Hardening", false, "MED", "High", "EASY", fmt.Sprintf("Privilege enabled via values: %s=true", k),
					"Values overrides can silently enable dangerous settings.",
					"Set this value to false.", "", 10)
			}
		}
	}

	
	if len(history) > 0 {

		timeSinceLast := time.Since(history[0].Info.FirstDeployed.Time)
		if timeSinceLast < 24 * time.Hour {
			report.RiskVelocity = "Increasing"
		} else {
			report.RiskVelocity = "Stable"
		}
	}

	
	report.ScoreRBAC = normalizeScore(report.ScoreRBAC)
	report.ScoreHardening = normalizeScore(report.ScoreHardening)
	report.ScoreImages = normalizeScore(report.ScoreImages)
	report.ScoreSecrets = normalizeScore(report.ScoreSecrets)
	report.ScoreExposure = normalizeScore(report.ScoreExposure)
	report.ScoreSupply = normalizeScore(report.ScoreSupply)


	totalScore := (float64(report.ScoreRBAC)*1.2 +
		float64(report.ScoreHardening)*1.2 +
		float64(report.ScoreImages)*1.0 +
		float64(report.ScoreSecrets)*1.0 +
		float64(report.ScoreExposure)*0.8 +
		float64(report.ScoreSupply)*0.8)
	
	avg := int(totalScore / 6.0)
	report.Score = avg

	
	switch {
	case avg >= 95:
		report.Grade = "A+"
	case avg >= 90:
		report.Grade = "A"
	case avg >= 80:
		report.Grade = "B"
	case avg >= 70:
		report.Grade = "C"
	case avg >= 60:
		report.Grade = "D"
	default:
		report.Grade = "F"
	}

	
	if blast.Score > 5.0 {
		blast.Score = 5.0
	}
	report.BlastRadiusInfo = blast


	sort.Slice(report.Findings, func(i, j int) bool {
		si := scoreSev(report.Findings[i].Severity)
		sj := scoreSev(report.Findings[j].Severity)
		if si != sj { return si > sj } 
		
		ei := scoreEffort(report.Findings[i].Effort)
		ej := scoreEffort(report.Findings[j].Effort)
		return ei > ej 
	})

	
	if len(report.Findings) > 0 {
		report.NextBestAction = &report.Findings[0]
	}

	return report
}


func normalizeScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}


func flattenValues(m map[string]interface{}, prefix string) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		if nextMap, ok := v.(map[string]interface{}); ok {
			sub := flattenValues(nextMap, fullKey)
			for sk, sv := range sub {
				out[sk] = sv
			}
		} else {
			out[fullKey] = v
		}
	}
	return out
}

func scoreSev(s string) int {
	switch s {
	case "CRITICAL": return 4
	case "HIGH": return 3
	case "MED": return 2
	case "LOW": return 1
	default: return 0
	}
}

func scoreEffort(e string) int {
	switch e {
	case "EASY": return 3
	case "MED": return 2
	case "HARD": return 1
	default: return 0
	}
}