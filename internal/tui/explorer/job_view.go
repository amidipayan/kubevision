package explorer

import (
	"fmt"
	"sort"
	"time"

	"github.com/amidipayan/kubevision/internal/utils"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listers "k8s.io/client-go/listers/batch/v1"
)



type JobView struct {
	lister listers.JobLister
}

func NewJobView(lister listers.JobLister) *JobView {
	return &JobView{lister: lister}
}

func (j *JobView) Title() string {
	return "Jobs"
}


func (j *JobView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
}

func (j *JobView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "COMPLETIONS", "DURATION", "AGE"}, []int{45, 15, 15, 15, 10}
}

func (j *JobView) Retrieve(namespace string) ([]Resource, error) {
	var items []*batchv1.Job
	var err error

	if namespace == "" {
		items, err = j.lister.List(labels.Everything())
	} else {
		items, err = j.lister.Jobs(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, job := range items {
		
		duration := "-"
		if job.Status.StartTime != nil {
			end := time.Now()
			if job.Status.CompletionTime != nil {
				end = job.Status.CompletionTime.Time
			}
			d := end.Sub(job.Status.StartTime.Time).Round(time.Second)
			duration = d.String()
		}

		status := fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions)
		
		if job.Spec.Completions == nil {
			status = fmt.Sprintf("%d/-", job.Status.Succeeded)
		}

		uiResources = append(uiResources, Resource{
			Name:      job.Name,
			Namespace: job.Namespace,
			Status:    status, 
			Kind:      "Job",
			Age:       utils.ComputeAge(&job.CreationTimestamp),
			AgeRaw:    job.CreationTimestamp.Time,
			Extras:    []string{duration},
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].AgeRaw.After(uiResources[j].AgeRaw) 
	})

	return uiResources, nil
}



type CronJobView struct {
	lister listers.CronJobLister
}

func NewCronJobView(lister listers.CronJobLister) *CronJobView {
	return &CronJobView{lister: lister}
}

func (c *CronJobView) Title() string {
	return "CronJobs"
}


func (c *CronJobView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
}

func (c *CronJobView) Headers() ([]string, []int) {
	
	return []string{"NAME", "NAMESPACE", "SCHEDULE", "SUSPEND", "ACTIVE", "LAST SCHED", "AGE"}, []int{35, 15, 15, 10, 8, 15, 10}
}

func (c *CronJobView) Retrieve(namespace string) ([]Resource, error) {
	var items []*batchv1.CronJob
	var err error

	if namespace == "" {
		items, err = c.lister.List(labels.Everything())
	} else {
		items, err = c.lister.CronJobs(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, cj := range items {
		lastSched := "-"
		if cj.Status.LastScheduleTime != nil {
			lastSched = utils.ComputeAge(cj.Status.LastScheduleTime)
		}
		
		suspend := "False"
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			suspend = "True"
		}

		uiResources = append(uiResources, Resource{
			Name:      cj.Name,
			Namespace: cj.Namespace,
			Status:    suspend, 
			Kind:      "CronJob",
			Age:       utils.ComputeAge(&cj.CreationTimestamp),
			AgeRaw:    cj.CreationTimestamp.Time,
			Extras: []string{
				cj.Spec.Schedule,
				suspend,
				fmt.Sprintf("%d", len(cj.Status.Active)),
				lastSched,
			},
		})
	}

	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].Name < uiResources[j].Name
	})

	return uiResources, nil
}