package helm

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)



type Severity string
type Category string
type Scope string

const (
	SevCritical Severity = "CRITICAL"
	SevHigh     Severity = "HIGH"
	SevMedium   Severity = "MEDIUM"
	SevLow      Severity = "LOW"
	SevInfo     Severity = "INFO"

	
	CatAvailability  Category = "Availability"
	CatReliability   Category = "Reliability"
	CatSecurity      Category = "Security"
	CatDeployment    Category = "Deployment Safety"
	CatPerformance   Category = "Performance & Capacity"
	CatOperability   Category = "Operability"
	CatObservability Category = "Observability"
	CatConfiguration Category = "Configuration"
	CatSLO           Category = "Service Level Objectives" 
	CatIncidents     Category = "Incident History"         

	ScopeCluster   Scope = "Cluster"
	ScopeNamespace Scope = "Namespace"
	ScopeWorkload  Scope = "Workload"
)


var BasePenalty = map[Severity]float64{
	SevCritical: 50.0,
	SevHigh:     25.0,
	SevMedium:   10.0,
	SevLow:      3.0,
	SevInfo:     0.0,
}


var CatWeight = map[Category]float64{
	CatSLO:           2.0, 
	CatIncidents:     1.8, 
	CatAvailability:  1.5, 
	CatReliability:   1.4, 
	CatSecurity:      1.3, 
	CatDeployment:    1.2, 
	CatPerformance:   1.2, 
	CatConfiguration: 1.0, 
	CatOperability:   1.0, 
	CatObservability: 1.0, 
}


var ScopeMultiplier = map[Scope]float64{
	ScopeCluster:   1.5, 
	ScopeNamespace: 1.2, 
	ScopeWorkload:  1.0, 
}



type HeuristicResult struct {
	Severity    Severity
	Category    Category
	Scope       Scope
	Title       string
	Description string
	Symptom     string
	Remediation string
	
	
	ScoreImpact float64 
	Calculation string  
	Timestamp   time.Time 
}

type SREAnalysis struct {
	Results           []HeuristicResult
	Score             int      
	SafetyGrade       string   
	PrimaryRiskDriver string   
	RiskFactors       []string 
	
	
	SLOStatus     string 
	IncidentTrend string 
}


type detectorContext struct {
	hasPDB            bool
	singleReplica     bool
	missingProbes     bool
	highRestarts      bool
	sloExhausted      bool
	recentFailure     bool
	
	
	highIssuesPerCat  map[Category]int
	availHighCount    int
	securityHighCount int
}


func AnalyzeReleaseHeuristics(history []*release.Release, objs []*unstructured.Unstructured) SREAnalysis {
	ctx := &detectorContext{
		highIssuesPerCat: make(map[Category]int),
	}
	report := SREAnalysis{Results: []HeuristicResult{}}

	
	var curr, prev *release.Release
	if len(history) > 0 {
		curr = history[0]
	}
	if len(history) > 1 {
		prev = history[1]
	}

	if curr == nil {
		return report
	}

	
	detectAvailability(objs, &report, ctx)
	detectReliability(objs, &report, ctx)
	detectPerformance(objs, &report, ctx)
	detectSecurity(curr, objs, &report, ctx)
	detectOpsObs(objs, &report, ctx)
	detectConfigurationStatic(objs, &report, ctx)


	if prev != nil {
		detectDeploymentSafety(curr, prev, &report, ctx)
		detectConfigurationDiff(curr, prev, &report)
	} else {
		addResult(&report, ctx, HeuristicResult{
			Severity:    SevLow,
			Category:    CatDeployment,
			Scope:       ScopeNamespace,
			Title:       "Limited Rollback History",
			Description: "No previous revision found (Fresh Install).",
			Symptom:     "Recovery options constrained.",
			Remediation: "Retain sufficient Helm revisions.",
		})
	}

	
	detectSLO(objs, &report, ctx)
	detectIncidents(history, &report, ctx)

	
	report.Score = calculateScore(&report, ctx)
	report.SafetyGrade = calculateGrade(report.Score)
	determinePrimaryDriver(&report, ctx)

	return report
}



func calculateScore(report *SREAnalysis, ctx *detectorContext) int {
	totalDeduction := 0.0

	
	for i := range report.Results {
		r := &report.Results[i]
		base := BasePenalty[r.Severity]
		weight := CatWeight[r.Category]
		scope := ScopeMultiplier[r.Scope]
		
		deduction := base * weight * scope
		r.ScoreImpact = deduction
		r.Calculation = fmt.Sprintf("%.0f(Base) × %.1f(Cat) × %.1f(Scope)", base, weight, scope)
		
		totalDeduction += deduction
	}


	for _, count := range ctx.highIssuesPerCat {
		if count > 1 {
			added := totalDeduction * 0.20
			totalDeduction += added
			report.RiskFactors = append(report.RiskFactors, "Compound: Multiple HIGH risks in single category (+20%)")
			break 
		}
	}

	
	if ctx.availHighCount > 0 && ctx.securityHighCount > 0 {
		added := totalDeduction * 0.25
		totalDeduction += added
		report.RiskFactors = append(report.RiskFactors, "Compound: Availability & Security Critical (+25%)")
	}

	
	if ctx.sloExhausted && totalDeduction > 15 {
		added := totalDeduction * 0.50
		totalDeduction += added
		report.RiskFactors = append(report.RiskFactors, "Critical: Deployment with Exhausted SLO (+50%)")
	}

	
	if ctx.recentFailure {
		added := totalDeduction * 0.30
		totalDeduction += added
		report.RiskFactors = append(report.RiskFactors, "Warning: Recurring Incident Pattern (+30%)")
	}

	
	if ctx.singleReplica && !ctx.hasPDB {
		added := totalDeduction * 0.20
		totalDeduction += added
		report.RiskFactors = append(report.RiskFactors, "Compound: Fragile Topology (1 Replica + No PDB) (+20%)")
	}

	finalScore := 100.0 - totalDeduction
	if finalScore < 0 { finalScore = 0 }
	return int(math.Round(finalScore))
}

func calculateGrade(score int) string {
	switch {
	case score >= 95: return "A+"
	case score >= 90: return "A"
	case score >= 80: return "B"
	case score >= 70: return "C"
	case score >= 60: return "D"
	default:          return "F"
	}
}

func determinePrimaryDriver(report *SREAnalysis, ctx *detectorContext) {
	if ctx.sloExhausted {
		report.PrimaryRiskDriver = "SLO Exhausted"
		report.SLOStatus = "Exhausted"
	} else if ctx.recentFailure {
		report.PrimaryRiskDriver = "Recurring Incident"
		report.IncidentTrend = "Unstable"
	} else if ctx.singleReplica {
		report.PrimaryRiskDriver = "Availability (Single Replica)"
	} else if ctx.missingProbes {
		report.PrimaryRiskDriver = "Reliability (No Probes)"
	} else if report.Score < 60 {
		report.PrimaryRiskDriver = "Critical Operational Risks"
	} else {
		report.PrimaryRiskDriver = "Stable"
		report.SLOStatus = "Healthy"
		report.IncidentTrend = "Stable"
	}
}

func addResult(report *SREAnalysis, ctx *detectorContext, r HeuristicResult) {
	if r.Severity == SevHigh || r.Severity == SevCritical {
		ctx.highIssuesPerCat[r.Category]++
		if r.Category == CatAvailability { ctx.availHighCount++ }
		if r.Category == CatSecurity { ctx.securityHighCount++ }
	}
	report.Results = append(report.Results, r)
}



func detectSLO(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {

	
	sloFound := false
	
	for _, obj := range objs {
		ann := obj.GetAnnotations()
		if ann == nil { continue }

		if target, ok := ann["sre.kubevision.io/slo-target"]; ok {
			sloFound = true
			
		
			if status, ok := ann["sre.kubevision.io/slo-status"]; ok {
				if status == "exhausted" || status == "burning" {
					ctx.sloExhausted = true
					addResult(report, ctx, HeuristicResult{
						Severity:    SevCritical,
						Category:    CatSLO,
						Scope:       ScopeWorkload,
						Title:       "Error Budget Exhausted",
						Description: fmt.Sprintf("SLO Target %s is currently failing.", target),
						Symptom:     "Deployment increases risk of violation.",
						Remediation: "Halt feature deployment. Focus on reliability fixes.",
					})
				} else {
					addResult(report, ctx, HeuristicResult{
						Severity:    SevInfo,
						Category:    CatSLO,
						Scope:       ScopeWorkload,
						Title:       "SLO Defined",
						Description: fmt.Sprintf("Target: %s | Status: %s", target, status),
						Symptom:     "Reliability is being measured.",
						Remediation: "Maintain current error budget.",
					})
				}
			}
		}
	}

	if !sloFound {
		addResult(report, ctx, HeuristicResult{
			Severity:    SevLow,
			Category:    CatSLO,
			Scope:       ScopeWorkload,
			Title:       "No SLO Definitions",
			Description: "Workload has no reliability targets defined.",
			Symptom:     "Cannot measure user impact objectively.",
			Remediation: "Define Service Level Objectives (SLOs) via annotations.",
		})
	}
}



func detectIncidents(history []*release.Release, report *SREAnalysis, ctx *detectorContext) {
	if len(history) < 2 { return }


	failCount := 0
	for _, rel := range history {
		if rel.Info.Status == release.StatusFailed || rel.Info.Status == release.StatusPendingRollback {
			failCount++
		}
	}

	if failCount > 1 {
		ctx.recentFailure = true
		addResult(report, ctx, HeuristicResult{
			Severity:    SevHigh,
			Category:    CatIncidents,
			Scope:       ScopeWorkload,
			Title:       "High Failure Rate Detected",
			Description: fmt.Sprintf("%d of last %d revisions failed.", failCount, len(history)),
			Symptom:     "Instability pattern observed in history.",
			Remediation: "Freeze deployments and investigate root cause.",
		})
	}

	
	curr := history[0]
	currHash := computeConfigHash(curr)
	
	for i := 1; i < len(history); i++ {
		old := history[i]
		if (old.Info.Status == release.StatusFailed) && (computeConfigHash(old) == currHash) {
			addResult(report, ctx, HeuristicResult{
				Severity:    SevCritical,
				Category:    CatIncidents,
				Scope:       ScopeWorkload,
				Title:       "Regression: Known Bad Config",
				Description: fmt.Sprintf("Current config matches failed revision %d.", old.Version),
				Symptom:     "Re-deploying a known broken state.",
				Remediation: "Revert values/chart version immediately.",
			})
			break
		}
	}
}

func computeConfigHash(r *release.Release) string {
	
	out, _ := yaml.Marshal(r.Config)
	return fmt.Sprintf("%s-%s", r.Chart.Metadata.Version, string(out))
}



func detectAvailability(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	workloadFound := false
	for _, obj := range objs {
		kind := obj.GetKind()
		if kind == "PodDisruptionBudget" { ctx.hasPDB = true }
		if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
			workloadFound = true
			replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
			if !found { replicas = 1 }

			if replicas == 1 && kind != "DaemonSet" {
				ctx.singleReplica = true
				addResult(report, ctx, HeuristicResult{
					Severity:    SevMedium,
					Category:    CatAvailability,
					Scope:       ScopeWorkload,
					Title:       "Single Replica Detected",
					Description: fmt.Sprintf("%s/%s has 1 replica.", kind, obj.GetName()),
					Symptom:     "Downtime possible during crashes or upgrades.",
					Remediation: "Increase replicas to ≥ 2 and configure podAntiAffinity.",
				})
			}
			
			
			ts, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "topologySpreadConstraints")
			if !found || len(ts) == 0 {
				addResult(report, ctx, HeuristicResult{
					Severity:    SevLow,
					Category:    CatAvailability,
					Scope:       ScopeWorkload,
					Title:       "No topologySpreadConstraints",
					Description: "Pods may co-locate on the same node/zone.",
					Symptom:     "Reduced resilience against zonal failures.",
					Remediation: "Distribute replicas across zones.",
				})
			}
		}
	}
	if workloadFound && !ctx.hasPDB {
		addResult(report, ctx, HeuristicResult{
			Severity:    SevMedium,
			Category:    CatAvailability,
			Scope:       ScopeNamespace,
			Title:       "PodDisruptionBudget Missing",
			Description: "No PDB found for workloads.",
			Symptom:     "Node drain or voluntary eviction may cause total outage.",
			Remediation: "Add PDB with minAvailable.",
		})
	}
}



func detectReliability(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	for _, obj := range objs {
		kind := obj.GetKind()
		if kind == "Deployment" || kind == "StatefulSet" {
			containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			for _, c := range containers {
				ctr, _ := c.(map[string]interface{})
				name := ctr["name"].(string)
				_, hasReady := ctr["readinessProbe"]
				_, hasLive := ctr["livenessProbe"]

				if !hasReady {
					ctx.missingProbes = true
					addResult(report, ctx, HeuristicResult{
						Severity:    SevMedium,
						Category:    CatReliability,
						Scope:       ScopeWorkload,
						Title:       "No Readiness Probe",
						Description: fmt.Sprintf("Container '%s' missing readiness check.", name),
						Symptom:     "Traffic sent to unready pods.",
						Remediation: "Add readinessProbe.",
					})
				}
				if !hasLive {
					addResult(report, ctx, HeuristicResult{
						Severity:    SevLow,
						Category:    CatReliability,
						Scope:       ScopeWorkload,
						Title:       "No Liveness Probe",
						Description: fmt.Sprintf("Container '%s' missing liveness check.", name),
						Symptom:     "Cannot auto-heal deadlocks.",
						Remediation: "Add livenessProbe.",
					})
				}
			}
		}
		
		if kind == "Pod" {
			statuses, _, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
			for _, s := range statuses {
				status, _ := s.(map[string]interface{})
				restarts, _, _ := unstructured.NestedInt64(status, "restartCount")
				if restarts > 5 {
					addResult(report, ctx, HeuristicResult{
						Severity:    SevLow,
						Category:    CatReliability,
						Scope:       ScopeWorkload,
						Title:       "Restart Count Increasing",
						Description: fmt.Sprintf("Pod %s has %d restarts.", obj.GetName(), restarts),
						Symptom:     "Hidden instability.",
						Remediation: "Investigate logs/events.",
					})
				}
			}
		}
	}
}



func detectPerformance(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	for _, obj := range objs {
		if obj.GetKind() == "Deployment" {
			containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			for _, c := range containers {
				ctr, _ := c.(map[string]interface{})
				res, _ := ctr["resources"].(map[string]interface{})
				req, _ := res["requests"].(map[string]interface{})
				
				if len(req) == 0 {
					addResult(report, ctx, HeuristicResult{
						Severity:    SevMedium,
						Category:    CatPerformance,
						Scope:       ScopeWorkload,
						Title:       "No Resource Requests",
						Description: "Container has no guaranteed CPU/Mem.",
						Symptom:     "Scheduler starvation.",
						Remediation: "Set resource requests.",
					})
				}
			}
		}
	}
}



func detectSecurity(rel *release.Release, objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	
	valBytes, _ := yaml.Marshal(rel.Config)
	if strings.Contains(strings.ToLower(string(valBytes)), "password:") {
		addResult(report, ctx, HeuristicResult{
			Severity:    SevLow,
			Category:    CatSecurity,
			Scope:       ScopeWorkload,
			Title:       "Potential Secrets in Values",
			Description: "Keywords 'password' found in values.yaml.",
			Symptom:     "Credential leak risk.",
			Remediation: "Use Kubernetes Secrets.",
		})
	}

	for _, obj := range objs {
		if obj.GetKind() == "ClusterRoleBinding" {
			addResult(report, ctx, HeuristicResult{
				Severity:    SevHigh,
				Category:    CatSecurity,
				Scope:       ScopeCluster,
				Title:       "ClusterRoleBinding Used",
				Description: fmt.Sprintf("%s grants global privileges.", obj.GetName()),
				Symptom:     "Excessive blast radius.",
				Remediation: "Use namespaced RoleBinding.",
			})
		}
		
		if obj.GetKind() == "Deployment" {
			containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			for _, c := range containers {
				ctr, _ := c.(map[string]interface{})
				sec, found, _ := unstructured.NestedMap(ctr, "securityContext")
				if found {
					if runAs, ok := sec["runAsUser"].(int64); ok && runAs == 0 {
						addResult(report, ctx, HeuristicResult{
							Severity:    SevHigh,
							Category:    CatSecurity,
							Scope:       ScopeWorkload,
							Title:       "Running as Root",
							Description: "Container explicitly set UID 0.",
							Symptom:     "Increased attack surface.",
							Remediation: "Run as non-root.",
						})
					}
				}
			}
		}
	}
}



func detectOpsObs(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	for _, obj := range objs {
		if obj.GetKind() == "Deployment" {
			
			ann := obj.GetAnnotations()
			if ann == nil || ann["owner"] == "" {
				addResult(report, ctx, HeuristicResult{
					Severity:    SevMedium,
					Category:    CatOperability,
					Scope:       ScopeWorkload,
					Title:       "No Ownership Labels",
					Description: "Missing owner annotation.",
					Symptom:     "On-call confusion.",
					Remediation: "Add 'owner' annotation.",
				})
			}
			
			spec, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
			if !strings.Contains(fmt.Sprintf("%v", spec), "prometheus") {
				addResult(report, ctx, HeuristicResult{
					Severity:    SevMedium,
					Category:    CatObservability,
					Scope:       ScopeWorkload,
					Title:       "No Metrics Endpoint",
					Description: "No prometheus annotations found.",
					Symptom:     "Blind during incidents.",
					Remediation: "Expose metrics.",
				})
			}
		}
	}
}



func detectDeploymentSafety(curr, prev *release.Release, report *SREAnalysis, ctx *detectorContext) {
	if curr.Chart.Metadata.AppVersion != prev.Chart.Metadata.AppVersion {
		addResult(report, ctx, HeuristicResult{
			Severity:    SevMedium,
			Category:    CatDeployment,
			Scope:       ScopeWorkload,
			Title:       "App Version Change",
			Description: fmt.Sprintf("%s -> %s", prev.Chart.Metadata.AppVersion, curr.Chart.Metadata.AppVersion),
			Symptom:     "Full rolling restart.",
			Remediation: "Monitor error rates.",
		})
	}
	
	
	currBytes, _ := yaml.Marshal(curr.Config)
	prevBytes, _ := yaml.Marshal(prev.Config)
	currStr := string(currBytes)
	prevStr := string(prevBytes)

	riskKeys := []string{"ingress", "service", "port", "host"}
	for _, key := range riskKeys {
		k := key + ":"
		if strings.Contains(currStr, k) && !strings.Contains(prevStr, k) {
			addResult(report, ctx, HeuristicResult{
				Severity:    SevMedium,
				Category:    CatDeployment,
				Scope:       ScopeCluster,
				Title:       "Values Change Impacts Traffic",
				Description: fmt.Sprintf("Key '%s' added/modified in values.", key),
				Symptom:     "Traffic routing behavior may change.",
				Remediation: "Verify endpoints and validate post-upgrade traffic.",
			})
		}
	}
}



func detectConfigurationStatic(objs []*unstructured.Unstructured, report *SREAnalysis, ctx *detectorContext) {
	for _, obj := range objs {
		if obj.GetKind() == "Deployment" {
			vols, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
			if found {
				for _, v := range vols {
					vol, _ := v.(map[string]interface{})
					if _, ok := vol["configMap"]; ok {
						addResult(report, ctx, HeuristicResult{
							Severity:    SevLow,
							Category:    CatConfiguration,
							Scope:       ScopeWorkload,
							Title:       "ConfigMap Mount Trigger",
							Description: "Pod mounts ConfigMap.",
							Symptom:     "Reloads might cause instability.",
							Remediation: "Verify reload behavior.",
						})
						break
					}
				}
			}
		}
	}
}

func detectConfigurationDiff(curr, prev *release.Release, report *SREAnalysis) {
	currS := fmt.Sprintf("%v", curr.Config)
	prevS := fmt.Sprintf("%v", prev.Config)
	if strings.Contains(currS, "env:") && !strings.Contains(prevS, "env:") {
		dummyCtx := &detectorContext{}
		addResult(report, dummyCtx, HeuristicResult{
			Severity:    SevLow,
			Category:    CatConfiguration,
			Scope:       ScopeWorkload,
			Title:       "Environment Variables Changed",
			Description: "Env vars modified.",
			Symptom:     "Runtime behavior change.",
			Remediation: "Validate runtime.",
		})
	}
}