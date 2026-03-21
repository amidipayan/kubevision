package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)


type ConfigInfo struct {
	Context string
	Cluster string
	User    string
	IDE     string
}


type KubeClient struct {
	clientset     *kubernetes.Clientset
	metricsClient *metrics.Clientset
	dynamicClient dynamic.Interface 
	mapper        meta.RESTMapper   
	restConfig    *rest.Config
	configInfo    ConfigInfo

	
	rawConfig    clientcmdapi.Config
	loadingRules *clientcmd.ClientConfigLoadingRules
	overrides    *clientcmd.ConfigOverrides
}


func NewKubeClient() (*KubeClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}

	return buildClient(loadingRules, configOverrides)
}


func buildClient(rules *clientcmd.ClientConfigLoadingRules, overrides *clientcmd.ConfigOverrides) (*KubeClient, error) {
	
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules,
		overrides,
	)

	
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client config: %w", err)
	}

	
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	
	metricsClient, err := metrics.NewForConfig(restConfig)
	if err != nil {
		metricsClient = nil
	}

	
	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Dynamic client: %w", err)
	}

	
	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	
	rawConfig, err := config.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw KubeConfig: %w", err)
	}

	
	info := ConfigInfo{
		Context: rawConfig.CurrentContext,
		IDE:     "Kubevision TUI",
	}

	currentCtx := rawConfig.Contexts[rawConfig.CurrentContext]
	if currentCtx != nil {
		info.Cluster = currentCtx.Cluster
		info.User = currentCtx.AuthInfo
	} else {
		info.Cluster = "Unknown Cluster"
		info.User = "Unknown User"
	}

	restConfig.Timeout = 30 * time.Second

	return &KubeClient{
		clientset:     clientset,
		metricsClient: metricsClient,
		dynamicClient: dynClient,
		mapper:        mapper,
		restConfig:    restConfig,
		configInfo:    info,
		rawConfig:     rawConfig,
		loadingRules:  rules,
		overrides:     overrides,
	}, nil
}


func (c *KubeClient) GetClientset() *kubernetes.Clientset {
	return c.clientset
}


func (c *KubeClient) GetMetricsClient() *metrics.Clientset {
	return c.metricsClient
}


func (c *KubeClient) GetRestConfig() *rest.Config {
	return c.restConfig
}


func (c *KubeClient) GetConfigInfo() ConfigInfo {
	return c.configInfo
}


func (c *KubeClient) ListContexts() []string {
	keys := make([]string, 0, len(c.rawConfig.Contexts))
	for k := range c.rawConfig.Contexts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}


func (c *KubeClient) SwitchContext(contextName string) error {
	newOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}
	newClient, err := buildClient(c.loadingRules, newOverrides)
	if err != nil {
		return err
	}
	c.clientset = newClient.clientset
	c.metricsClient = newClient.metricsClient
	c.dynamicClient = newClient.dynamicClient
	c.mapper = newClient.mapper
	c.restConfig = newClient.restConfig
	c.configInfo = newClient.configInfo
	c.rawConfig = newClient.rawConfig
	c.overrides = newOverrides
	return nil
}


func (c *KubeClient) ApplyYAML(namespace string, yamlData []byte) error {
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, gvk, err := decoder.Decode(yamlData, nil, obj)
	if err != nil {
		return fmt.Errorf("failed to decode YAML: %w", err)
	}

	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to find resource mapping for %s: %w", gvk.Kind, err)
	}

	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "status")

	targetNs := namespace
	if targetNs == "" {
		targetNs = obj.GetNamespace()
	}
	if targetNs == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		targetNs = "default"
	}

	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		dri = c.dynamicClient.Resource(mapping.Resource).Namespace(targetNs)
	} else {
		dri = c.dynamicClient.Resource(mapping.Resource)
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	force := true
	patchOptions := metav1.PatchOptions{
		FieldManager: "kubevision-tui",
		Force:        &force,
	}

	_, err = dri.Patch(context.TODO(), obj.GetName(), types.ApplyPatchType, data, patchOptions)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}

	return nil
}


func (c *KubeClient) IsHealthy() error {
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.clientset.Discovery().RESTClient().Get().AbsPath("/version").Do(ctx).Raw()
	return err
}


func (c *KubeClient) GetClusterStats() (string, string, error) {
	if c.metricsClient == nil {
		return "N/A", "N/A", fmt.Errorf("metrics client not initialized")
	}

	ctx := context.Background()
	nodeMetricsList, err := c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "Err", "Err", err
	}
	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "Err", "Err", err
	}

	var currentCPU, currentMem, totalCPU, totalMem int64
	for _, m := range nodeMetricsList.Items {
		currentCPU += m.Usage.Cpu().MilliValue()
		currentMem += m.Usage.Memory().Value()
	}
	for _, n := range nodeList.Items {
		totalCPU += n.Status.Allocatable.Cpu().MilliValue()
		totalMem += n.Status.Allocatable.Memory().Value()
	}

	if totalCPU == 0 || totalMem == 0 {
		return "0%", "0Mi", nil
	}

	cpuPercent := float64(currentCPU) / float64(totalCPU) * 100
	memUsageMiB := float64(currentMem) / (1024 * 1024)
	totalMemMiB := float64(totalMem) / (1024 * 1024)
	memPercent := (memUsageMiB / totalMemMiB) * 100

	cpuStr := fmt.Sprintf("%.1f%%", cpuPercent)
	memStr := fmt.Sprintf("%dMi (%.0f%%)", int64(memUsageMiB), memPercent)

	return cpuStr, memStr, nil
}


func (c *KubeClient) ScaleResource(namespace, kind, name string, delta int) (int32, error) {
	var err error
	var current, desired int32
	ctx := context.TODO()

	switch kind {
	case "Deployment":
		scale, err := c.clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		current = scale.Spec.Replicas
		desired = current + int32(delta)
		if desired < 0 {
			desired = 0
		}
		scale.Spec.Replicas = desired
		_, err = c.clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	case "ReplicaSet":
		scale, err := c.clientset.AppsV1().ReplicaSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		current = scale.Spec.Replicas
		desired = current + int32(delta)
		if desired < 0 {
			desired = 0
		}
		scale.Spec.Replicas = desired
		_, err = c.clientset.AppsV1().ReplicaSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	case "StatefulSet":
		scale, err := c.clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		current = scale.Spec.Replicas
		desired = current + int32(delta)
		if desired < 0 {
			desired = 0
		}
		scale.Spec.Replicas = desired
		_, err = c.clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	default:
		return 0, fmt.Errorf("scaling not supported for %s", kind)
	}
	return desired, err
}

func (c *KubeClient) RestartResource(namespace, kind, name string) error {
	timestamp := time.Now().Format(time.RFC3339)
	patchData := fmt.Sprintf(`{"spec": {"template": {"metadata": {"annotations": {"kubectl.kubernetes.io/restartedAt": "%s"}}}}}`, timestamp)
	var err error
	ctx := context.TODO()
	opts := metav1.PatchOptions{FieldManager: "kubevision"}

	switch kind {
	case "Deployment":
		_, err = c.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patchData), opts)
	case "StatefulSet":
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patchData), opts)
	case "DaemonSet":
		_, err = c.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patchData), opts)
	default:
		return fmt.Errorf("restart not supported for %s", kind)
	}
	return err
}

func (c *KubeClient) ParseManifestToObjects(manifest string) ([]*unstructured.Unstructured, error) {
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	reader := k8syaml.NewYAMLReader(bufio.NewReader(strings.NewReader(manifest)))
	var objs []*unstructured.Unstructured

	for {
		raw, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}
		_, _, err = decoder.Decode(raw, nil, obj)
		if err != nil {
			continue
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func (c *KubeClient) DiscoverResources() ([]schema.GroupVersionResource, error) {
	
	lists, err := c.clientset.Discovery().ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		return nil, err
	}

	var resources []schema.GroupVersionResource

	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}

		for _, r := range list.APIResources {
			
			if strings.Contains(r.Name, "/") {
				continue
			}

			canList := false
			for _, verb := range r.Verbs {
				if verb == "list" {
					canList = true
					break
				}
			}
			if !canList {
				continue
			}

			resources = append(resources, schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: r.Name,
			})
		}
	}

	
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Resource < resources[j].Resource
	})

	return resources, nil
}


func (c *KubeClient) ListTable(gvr schema.GroupVersionResource, namespace string) (*metav1.Table, error) {
	
	conf := *c.restConfig
	conf.GroupVersion = &schema.GroupVersion{Group: gvr.Group, Version: gvr.Version}
	conf.APIPath = "/apis"
	if gvr.Group == "" {
		conf.APIPath = "/api"
	}
	conf.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	client, err := rest.RESTClientFor(&conf)
	if err != nil {
		return nil, err
	}

	req := client.Get().
		Resource(gvr.Resource).
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json;as=Table;v=v1")

	if namespace != "" {
		req = req.Namespace(namespace)
	}

	
	result := req.Do(context.TODO())
	if result.Error() != nil {
		return nil, result.Error()
	}

	rawBody, err := result.Raw()
	if err != nil {
		return nil, err
	}

	
	table := &metav1.Table{}
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	if _, _, err := decoder.Decode(rawBody, nil, table); err != nil {
		return nil, fmt.Errorf("failed to decode Table: %w", err)
	}

	return table, nil
}



func (c *KubeClient) NewHelmConfiguration(namespace string) (*action.Configuration, error) {
	getter := &SimpleRESTClientGetter{Client: c, Namespace: namespace}
	cfg := new(action.Configuration)
	err := cfg.Init(getter, namespace, "secrets", func(format string, v ...interface{}) {})
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

type SimpleRESTClientGetter struct {
	Client    *KubeClient
	Namespace string
}

var _ genericclioptions.RESTClientGetter = (*SimpleRESTClientGetter)(nil)

func (g *SimpleRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.Client.restConfig, nil
}

func (g *SimpleRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.Client.restConfig)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (g *SimpleRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return g.Client.mapper, nil
}

func (g *SimpleRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		g.Client.loadingRules,
		g.Client.overrides,
	)
}

func (c *KubeClient) GetDynamicClient() dynamic.Interface { return c.dynamicClient }
func (c *KubeClient) GetMapper() meta.RESTMapper          { return c.mapper }