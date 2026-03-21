package auth

import (
	"fmt"
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/listers/rbac/v1"
)

type SubjectRef struct {
	Kind      string
	Name      string
	Namespace string
}

type SubjectSummary struct {
	Subject          SubjectRef
	IsAdmin          bool
	ClusterRoleCount int
	RoleCount        int
}

type PolicyRow struct {
	Resource string
	APIGroup string

	Get     bool
	List    bool
	Watch   bool
	Create  bool
	Patch   bool
	Update  bool
	Delete  bool
	DelList bool
}

type RBACScanner struct {
	roleLister        v1.RoleLister
	clusterRoleLister v1.ClusterRoleLister
	rbLister          v1.RoleBindingLister
	crbLister         v1.ClusterRoleBindingLister
}

func NewRBACScanner(factory informers.SharedInformerFactory) *RBACScanner {
	return &RBACScanner{
		roleLister:        factory.Rbac().V1().Roles().Lister(),
		clusterRoleLister: factory.Rbac().V1().ClusterRoles().Lister(),
		rbLister:          factory.Rbac().V1().RoleBindings().Lister(),
		crbLister:         factory.Rbac().V1().ClusterRoleBindings().Lister(),
	}
}

func (s *RBACScanner) FetchAggregatedRules(subjects []SubjectRef) ([]*PolicyRow, error) {
	rulesMap := make(map[string]*PolicyRow)

	mergeRule := func(rule rbacv1.PolicyRule) {
		if len(rule.Resources) == 0 {
			return 
		}

		targetResources := rule.Resources
		targetGroups := rule.APIGroups
		if len(targetGroups) == 0 {
			targetGroups = []string{""}
		}

		allVerbs := false
		for _, v := range rule.Verbs {
			if v == "*" {
				allVerbs = true
				break
			}
		}

		for _, res := range targetResources {
			for _, grp := range targetGroups {
				
				key := fmt.Sprintf("%s|%s", res, grp)
				
				if _, exists := rulesMap[key]; !exists {
					rulesMap[key] = &PolicyRow{
						Resource: res,
						APIGroup: grp,
					}
				}
				row := rulesMap[key]

				if allVerbs {
					row.Get, row.List, row.Watch = true, true, true
					row.Create, row.Patch, row.Update = true, true, true
					row.Delete, row.DelList = true, true
				} else {
					for _, v := range rule.Verbs {
						switch strings.ToLower(v) {
						case "get":
							row.Get = true
						case "list":
							row.List = true
						case "watch":
							row.Watch = true
						case "create":
							row.Create = true
						case "patch":
							row.Patch = true
						case "update":
							row.Update = true
						case "delete":
							row.Delete = true
						case "deletecollection":
							row.DelList = true
						}
					}
				}
			}
		}
	}

	
	crbs, _ := s.crbLister.List(labels.Everything())
	for _, b := range crbs {
		if matchesAnySubject(b.Subjects, subjects) {
			role, err := s.clusterRoleLister.Get(b.RoleRef.Name)
			if err == nil {
				for _, rule := range role.Rules {
					mergeRule(rule)
				}
			}
		}
	}

	
	rbs, _ := s.rbLister.List(labels.Everything())
	for _, b := range rbs {
		if matchesAnySubject(b.Subjects, subjects) {
			var rules []rbacv1.PolicyRule
			if b.RoleRef.Kind == "ClusterRole" {
				if cr, err := s.clusterRoleLister.Get(b.RoleRef.Name); err == nil {
					rules = cr.Rules
				}
			} else {
				if r, err := s.roleLister.Roles(b.Namespace).Get(b.RoleRef.Name); err == nil {
					rules = r.Rules
				}
			}
			for _, rule := range rules {
				mergeRule(rule)
			}
		}
	}

	
	var results []*PolicyRow
	for _, row := range rulesMap {
		results = append(results, row)
	}

	
	sort.Slice(results, func(i, j int) bool {
		if results[i].Resource == "*" && results[j].Resource != "*" { return true }
		if results[i].Resource != "*" && results[j].Resource == "*" { return false }
		if results[i].Resource == results[j].Resource {
			return results[i].APIGroup < results[j].APIGroup
		}
		return results[i].Resource < results[j].Resource
	})

	return results, nil
}


func (s *RBACScanner) ListSubjects() ([]SubjectSummary, error) {
	subjectMap := make(map[string]*SubjectSummary)
	getKey := func(sub rbacv1.Subject) string {
		return fmt.Sprintf("%s|%s|%s", sub.Kind, sub.Namespace, sub.Name)
	}

	crbs, _ := s.crbLister.List(labels.Everything())
	for _, b := range crbs {
		for _, sub := range b.Subjects {
			key := getKey(sub)
			if _, ok := subjectMap[key]; !ok {
				subjectMap[key] = &SubjectSummary{
					Subject: SubjectRef{Kind: sub.Kind, Name: sub.Name, Namespace: sub.Namespace},
				}
			}
			subjectMap[key].ClusterRoleCount++
			if b.RoleRef.Name == "cluster-admin" {
				subjectMap[key].IsAdmin = true
			}
		}
	}

	rbs, _ := s.rbLister.List(labels.Everything())
	for _, b := range rbs {
		for _, sub := range b.Subjects {
			key := getKey(sub)
			if _, ok := subjectMap[key]; !ok {
				subjectMap[key] = &SubjectSummary{
					Subject: SubjectRef{Kind: sub.Kind, Name: sub.Name, Namespace: sub.Namespace},
				}
			}
			subjectMap[key].RoleCount++
		}
	}

	var result []SubjectSummary
	for _, summary := range subjectMap {
		result = append(result, *summary)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Subject.Name < result[j].Subject.Name
	})
	return result, nil
}

func matchesAnySubject(bindingSubjects []rbacv1.Subject, requestedSubjects []SubjectRef) bool {
	for _, bSub := range bindingSubjects {
		for _, reqSub := range requestedSubjects {
			if bSub.Kind == reqSub.Kind && bSub.Name == reqSub.Name {
				if bSub.Kind == "ServiceAccount" {
					if bSub.Namespace == reqSub.Namespace {
						return true
					}
				} else {
					return true
				}
			}
		}
	}
	return false
}