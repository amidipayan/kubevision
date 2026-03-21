package informer

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/amidipayan/kubevision/internal/k8s/client"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)


type ResourceChangeMsg struct{}


type Manager struct {

	StaticFactory informers.SharedInformerFactory


	DynamicFactory dynamicinformer.DynamicSharedInformerFactory

	client *client.KubeClient
	stopCh chan struct{}


	activeWatches map[schema.GroupVersionResource]bool
	mu            sync.Mutex


	updates chan tea.Msg
}


func NewManager(c *client.KubeClient) *Manager {

	staticFactory := informers.NewSharedInformerFactory(c.GetClientset(), 10*time.Minute)

	
	dynamicFactory := dynamicinformer.NewDynamicSharedInformerFactory(c.GetDynamicClient(), 10*time.Minute)

	return &Manager{
		StaticFactory:  staticFactory,
		DynamicFactory: dynamicFactory,
		client:         c,
		stopCh:         make(chan struct{}),
		activeWatches:  make(map[schema.GroupVersionResource]bool),
		updates:        make(chan tea.Msg, 1), 
	}
}


func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	
	if m.stopCh != nil {
		close(m.stopCh)
	}


	m.stopCh = make(chan struct{})


	m.activeWatches = make(map[schema.GroupVersionResource]bool)
}


func (m *Manager) Start() {
	
	m.StaticFactory.Start(m.stopCh)
	m.StaticFactory.WaitForCacheSync(m.stopCh)

	
	m.DynamicFactory.Start(m.stopCh)
	m.DynamicFactory.WaitForCacheSync(m.stopCh)
}


func (m *Manager) WatchResource(gvr schema.GroupVersionResource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	
	if m.activeWatches[gvr] {
		return
	}

	
	inf := m.DynamicFactory.ForResource(gvr).Informer()

	
	m.Watch(inf)


	go inf.Run(m.stopCh)

	m.activeWatches[gvr] = true
}

func (m *Manager) Watch(inf cache.SharedIndexInformer) {
	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			m.notify()
		},
		UpdateFunc: func(old, new interface{}) {
			m.notify()
		},
		DeleteFunc: func(obj interface{}) {
			m.notify()
		},
	})
}


func (m *Manager) notify() {
	select {
	case m.updates <- ResourceChangeMsg{}:
	default:

	}
}

func (m *Manager) WaitForUpdates() tea.Msg {
	return <-m.updates
}