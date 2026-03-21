package explorer

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/apimachinery/pkg/runtime/schema"
)


type ResourceView interface {

	Title() string

	
	Headers() ([]string, []int)

	
	Retrieve(namespace string) ([]Resource, error)


	GetGVR() schema.GroupVersionResource
}


type Actionable interface {
	ResourceView


	HandleKey(msg tea.KeyMsg, resource Resource) (bool, tea.Cmd)


	ExtraHelp() map[string]string
}


type Resource struct {
	Name      string
	Namespace string
	Status    string
	Age       string
	Kind      string

	
	IP string

	
	CPU    string
	Memory string

	
	Extras []string

	
	AgeRaw    time.Time
	Restarts  int
	CPURaw    int64
	MemoryRaw int64
}