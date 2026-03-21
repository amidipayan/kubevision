package portforward

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)


type ActiveForward struct {
	ID         string 
	PodName    string
	Namespace  string
	LocalPort  string
	RemotePort string
	StopCh     chan struct{}
	ReadyCh    chan struct{}
	Error      error
	Active     bool
}


type Manager struct {
	client   *client.KubeClient
	forwards map[string]*ActiveForward
	mu       sync.Mutex 
}

func NewManager(c *client.KubeClient) *Manager {
	return &Manager{
		client:   c,
		forwards: make(map[string]*ActiveForward),
	}
}


func (m *Manager) Start(podName, namespace, ports string) (*ActiveForward, error) {
	parts := strings.Split(ports, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port format, expected LOCAL:REMOTE (e.g., 8080:80)")
	}
	localPort, remotePort := parts[0], parts[1]

	
	restConfig := m.client.GetRestConfig()
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	hostIP := strings.TrimPrefix(restConfig.Host, "https://")
	hostIP = strings.TrimPrefix(hostIP, "http://")

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, err
	}

	targetURL := &url.URL{
		Scheme: "https",
		Path:   path,
		Host:   hostIP,
	}
	if !strings.HasPrefix(restConfig.Host, "https") {
		targetURL.Scheme = "http"
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", targetURL)

	
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})

	id := fmt.Sprintf("%s:%s", podName, localPort)
	pf := &ActiveForward{
		ID:         id,
		PodName:    podName,
		Namespace:  namespace,
		LocalPort:  localPort,
		RemotePort: remotePort,
		StopCh:     stopCh,
		ReadyCh:    readyCh,
		Active:     false,
	}

	
	m.mu.Lock()
	m.forwards[id] = pf
	m.mu.Unlock()

	
	go func() {
		select {
		case <-pf.ReadyCh:
			m.mu.Lock()
			pf.Active = true
			m.mu.Unlock()
		case <-pf.StopCh:
			return 
		}
	}()

	
	go func() {
		
		fw, err := portforward.New(dialer, []string{ports}, stopCh, readyCh, io.Discard, io.Discard)
		if err != nil {
			m.mu.Lock()
			pf.Error = err
			pf.Active = false
			m.mu.Unlock()
			return
		}

		
		if err := fw.ForwardPorts(); err != nil {
			m.mu.Lock()
			pf.Error = err
			pf.Active = false
			m.mu.Unlock()
		}
	}()

	return pf, nil
}


func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pf, exists := m.forwards[id]; exists {
		close(pf.StopCh)
		delete(m.forwards, id)
	}
}


func (m *Manager) List() []*ActiveForward {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := make([]*ActiveForward, 0, len(m.forwards))
	for _, v := range m.forwards {
		list = append(list, v)
	}
	return list
}