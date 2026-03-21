package helm

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
)


type ChartResult struct {
	Name        string
	RepoName    string
	Version     string
	AppVersion  string
	Description string
}

type RepoManager struct {
	settings *cli.EnvSettings
	
	indexCache []ChartResult
	mu         sync.RWMutex
	loaded     bool
}

func NewRepoManager() *RepoManager {
	return &RepoManager{
		settings: cli.New(),
	}
}


func (r *RepoManager) LoadIndex() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	repoFile := r.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		
		if os.IsNotExist(err) {
			r.indexCache = []ChartResult{}
			r.loaded = true
			return 0, nil
		}
		return 0, err
	}

	var allCharts []ChartResult

	
	for _, cfg := range f.Repositories {
	
		idxFile := r.settings.RepositoryCache + "/" + helmCacheFile(cfg.Name)
		if _, err := os.Stat(idxFile); err != nil {
			continue
		}

		idx, err := repo.LoadIndexFile(idxFile)
		if err != nil {
			continue
		}

		
		for name, versions := range idx.Entries {
			if len(versions) == 0 {
				continue
			}
			
			latest := versions[0]			
			allCharts = append(allCharts, ChartResult{
				Name:        name,
				RepoName:    cfg.Name,
				Version:     latest.Version,
				AppVersion:  latest.AppVersion,
				Description: latest.Description,
			})
		}
	}

	r.indexCache = allCharts
	r.loaded = true
	return len(allCharts), nil
}


func (r *RepoManager) SearchCharts(query string) ([]ChartResult, error) {
	r.mu.RLock()
	
	if !r.loaded {
		r.mu.RUnlock()
		_, err := r.LoadIndex()
		if err != nil {
			return nil, err
		}
		r.mu.RLock()
	}
	defer r.mu.RUnlock()

	if query == "" {
		return r.indexCache, nil
	}

	query = strings.ToLower(query)
	var filtered []ChartResult

	for _, c := range r.indexCache {
		
		if strings.Contains(strings.ToLower(c.Name), query) ||
			strings.Contains(strings.ToLower(c.RepoName), query) ||
			strings.Contains(strings.ToLower(c.Description), query) {
			filtered = append(filtered, c)
		}
		
		if len(filtered) > 500 {
			break
		}
	}

	return filtered, nil
}


func helmCacheFile(repoName string) string {
	return fmt.Sprintf("%s-index.yaml", repoName)
}