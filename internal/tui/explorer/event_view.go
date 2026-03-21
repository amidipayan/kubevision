package explorer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amidipayan/kubevision/internal/utils"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
)

type EventView struct {
	lister listers.EventLister
}

func NewEventView(factory informers.SharedInformerFactory) *EventView {
	return &EventView{
		lister: factory.Core().V1().Events().Lister(),
	}
}

func (e *EventView) Title() string {
	return "Events"
}


func (e *EventView) GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
}


func (e *EventView) Headers() ([]string, []int) {

	return []string{"AGE", "NAMESPACE", "TYPE", "REASON", "OBJECT", "MESSAGE"}, []int{10, 12, 12, 20, 30, 50}
}

func (e *EventView) Retrieve(namespace string) ([]Resource, error) {
	events, err := e.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var uiResources []Resource
	for _, evt := range events {
		if namespace != "" && evt.Namespace != namespace {
			continue
		}

		
		ts := evt.LastTimestamp.Time
		if ts.IsZero() {
			ts = evt.EventTime.Time
		}
		if ts.IsZero() {
			ts = evt.FirstTimestamp.Time
		}

		
		cleanMsg := strings.ReplaceAll(evt.Message, "\n", " ")
		
		
		objRef := fmt.Sprintf("%s/%s", strings.ToLower(evt.InvolvedObject.Kind), evt.InvolvedObject.Name)

		
		uiResources = append(uiResources, Resource{
			Name:      evt.InvolvedObject.Name, 
			Namespace: evt.Namespace,
			Status:    evt.Type, 
			Age:       utils.ComputeAge(&evt.LastTimestamp),
			AgeRaw:    ts,
			Kind:      "Event",
			
			Extras:    []string{evt.Type, evt.Reason, objRef, cleanMsg},
		})
	}

	
	sort.Slice(uiResources, func(i, j int) bool {
		return uiResources[i].AgeRaw.Before(uiResources[j].AgeRaw)
	})

	return uiResources, nil
}