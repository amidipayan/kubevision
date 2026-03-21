package explorer

import (
	"fmt"
	"sort"

	"github.com/amidipayan/kubevision/internal/k8s/helm"
	"k8s.io/apimachinery/pkg/runtime/schema"
)


type RepoView struct {
	manager *helm.RepoManager
}

func NewRepoView() *RepoView {
	return &RepoView{
		manager: helm.NewRepoManager(),
	}
}


func (r *RepoView) LoadIndex() (int, error) {
	return r.manager.LoadIndex()
}


func (r *RepoView) Title() string {
	return "Helm Charts"
}


func (r *RepoView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}


func (r *RepoView) Headers() ([]string, []int) {
	
	return []string{"CHART", "REPO", "VERSION", "APP VER", "DESCRIPTION"}, []int{35, 15, 10, 15, 50}
}


func (r *RepoView) Retrieve(query string) ([]Resource, error) {

	limit := -1
	if query == "default" || query == "all" || query == "" {
		query = ""
		limit = 50
	}

	
	results, err := r.manager.SearchCharts(query)
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	count := 0
	for _, chart := range results {
		if limit > 0 && count >= limit {
			break
		}

		uiResources = append(uiResources, Resource{
			Name:      chart.Name,
			Kind:      "Chart",
			Namespace: chart.RepoName, 
			Status:    chart.Version,  
			

			Extras: []string{
				chart.RepoName,
				chart.Version,
				chart.AppVersion,
				chart.Description,
			},
		})
		count++
	}

	
	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	
	if limit > 0 && len(results) > limit {
		uiResources = append(uiResources, Resource{
			Name:      "... (Type '/' to search more)",
			Kind:      "Hint",
			Namespace: "-",
			Status:    "-",
			Extras:    []string{"-", "-", "-", fmt.Sprintf("%d more charts hidden...", len(results)-limit)},
		})
	}

	return uiResources, nil
}