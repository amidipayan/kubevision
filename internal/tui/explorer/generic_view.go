package explorer

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/jsonpath"
)


type GenericView struct {
	client         *client.KubeClient
	gvr            schema.GroupVersionResource
	kind           string
	headers        []string
	widths         []int
	printerColumns []PrinterColumn
	crdLoaded      bool
}

type PrinterColumn struct {
	Name     string
	JSONPath string
	Type     string
}

func NewGenericView(c *client.KubeClient, gvr schema.GroupVersionResource, kind string) *GenericView {
	return &GenericView{
		client: c,
		gvr:    gvr,
		kind:   kind,
	}
}

func (g *GenericView) Title() string {
	if g.gvr.Group != "" {
		return fmt.Sprintf("%s.%s", g.gvr.Resource, g.gvr.Group)
	}
	return g.gvr.Resource
}

func (g *GenericView) GetGVR() schema.GroupVersionResource { return g.gvr }

func (g *GenericView) Headers() ([]string, []int) {
	if len(g.headers) == 0 {
		return []string{"NAME", "AGE"}, []int{30, 10}
	}
	return g.headers, g.widths
}


func (g *GenericView) Retrieve(namespace string) ([]Resource, error) {
	
	if !g.crdLoaded && g.gvr.Group != "" {
		g.loadCRDDefinition()
		g.crdLoaded = true
	}

	
	if len(g.printerColumns) > 0 {
		return g.retrieveWithColumns(namespace)
	}

	
	return g.retrieveWithTable(namespace)
}


func (g *GenericView) loadCRDDefinition() {
	
	crdName := fmt.Sprintf("%s.%s", g.gvr.Resource, g.gvr.Group)
	
	
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	obj, err := g.client.GetDynamicClient().Resource(crdGVR).Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		
		return
	}

	
	versions, found, _ := unstructured.NestedSlice(obj.Object, "spec", "versions")
	if !found {
		return
	}

	for _, v := range versions {
		verMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		name, _, _ := unstructured.NestedString(verMap, "name")
		if name == g.gvr.Version {
			
			cols, found, _ := unstructured.NestedSlice(verMap, "additionalPrinterColumns")
			if found {
				var parsedCols []PrinterColumn
				
			

				for _, c := range cols {
					cMap, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					colName, _, _ := unstructured.NestedString(cMap, "name")
					path, _, _ := unstructured.NestedString(cMap, "jsonPath")
					t, _, _ := unstructured.NestedString(cMap, "type")
					
					
					if colName != "Age" {
						parsedCols = append(parsedCols, PrinterColumn{
							Name:     strings.ToUpper(colName),
							JSONPath: path,
							Type:     t,
						})
					}
				}
				g.printerColumns = parsedCols
			}
			break
		}
	}
}


func (g *GenericView) retrieveWithColumns(namespace string) ([]Resource, error) {
	list, err := g.client.GetDynamicClient().Resource(g.gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	
	var headers []string
	headers = append(headers, "NAME")
	for _, col := range g.printerColumns {
		headers = append(headers, col.Name)
	}
	headers = append(headers, "AGE")

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h) + 2
	}

	var resources []Resource

	
	j := jsonpath.New("parser")
	j.AllowMissingKeys(true) 

	for _, item := range list.Items {
		res := Resource{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Kind:      g.kind,
			AgeRaw:    item.GetCreationTimestamp().Time,
			Age:       utils.ComputeAge(&metav1.Time{Time: item.GetCreationTimestamp().Time}),
			Extras:    make([]string, len(headers)), 
		}

		
		res.Extras[0] = res.Name

		for i, col := range g.printerColumns {
			
			path := col.JSONPath
			if !strings.HasPrefix(path, "{") {
				path = fmt.Sprintf("{%s}", path)
			}

			if err := j.Parse(path); err != nil {
				res.Extras[i+1] = "<err>"
				continue
			}

			buf := new(bytes.Buffer)
			if err := j.Execute(buf, item.Object); err != nil {
				res.Extras[i+1] = ""
			} else {
				res.Extras[i+1] = buf.String()
			}

			
			if len(res.Extras[i+1])+2 > widths[i+1] {
				widths[i+1] = len(res.Extras[i+1]) + 2
			}
		}

		
		res.Extras[len(headers)-1] = res.Age
		
		
		if len(res.Name)+2 > widths[0] {
			widths[0] = len(res.Name) + 2
		}

		resources = append(resources, res)
	}

	
	g.headers = headers
	g.widths = widths

	return resources, nil
}


func (g *GenericView) retrieveWithTable(namespace string) ([]Resource, error) {
	
	table, err := g.client.ListTable(g.gvr, namespace)
	if err != nil {
		return nil, err
	}

	
	metricMap := make(map[string]struct{ cpu, mem int64 })

	if g.kind == "Pod" || g.kind == "Node" {
		mc := g.client.GetMetricsClient()
		if mc != nil {
			if g.kind == "Pod" {
				list, _ := mc.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), metav1.ListOptions{})
				if list != nil {
					for _, m := range list.Items {
						var c, mem int64
						for _, cont := range m.Containers {
							c += cont.Usage.Cpu().MilliValue()
							mem += cont.Usage.Memory().Value()
						}
						metricMap[m.Namespace+"/"+m.Name] = struct{ cpu, mem int64 }{c, mem}
					}
				}
			} else if g.kind == "Node" {
				list, _ := mc.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
				if list != nil {
					for _, m := range list.Items {
						c := m.Usage.Cpu().MilliValue()
						mem := m.Usage.Memory().Value()
						metricMap[m.Name] = struct{ cpu, mem int64 }{c, mem}
					}
				}
			}
		}
	}

	
	var headers []string
	idxName, idxAge, idxNs, idxStatus := -1, -1, -1, -1

	for i, col := range table.ColumnDefinitions {
		header := strings.ToUpper(col.Name)
		headers = append(headers, header)
		if header == "NAME" {
			idxName = i
		}
		if header == "AGE" || header == "CREATIONTIMESTAMP" {
			idxAge = i
		}
		if header == "NAMESPACE" {
			idxNs = i
		}
		if header == "STATUS" || header == "READY" || header == "PHASE" {
			idxStatus = i
		}
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h) + 2
	}

	var uiResources []Resource

	for _, row := range table.Rows {
		res := Resource{Kind: g.kind, Extras: make([]string, len(headers))}

		for i, cell := range row.Cells {
			if i >= len(widths) {
				break
			}
			val := fmt.Sprintf("%v", cell)

			
			if g.kind == "Pod" || g.kind == "Node" {
				header := headers[i]
				key := res.Name
				if idxName != -1 && idxName < len(row.Cells) {
					key = fmt.Sprintf("%v", row.Cells[idxName])
				}

				if g.kind == "Pod" {
					ns := res.Namespace
					if ns == "" && idxNs != -1 && idxNs < len(row.Cells) {
						ns = fmt.Sprintf("%v", row.Cells[idxNs])
					}
					if ns == "" {
						ns = namespace
					}
					key = ns + "/" + key
				}

				if m, ok := metricMap[key]; ok {
					if strings.Contains(header, "CPU") {
						limit := int64(1000)
						val = utils.RenderUsageBar(m.cpu, limit, "%dm")
					} else if strings.Contains(header, "MEM") {
						limit := int64(512 * 1024 * 1024)
						val = utils.RenderUsageBar(m.mem/(1024*1024), limit/(1024*1024), "%dMi")
					}
				}
			}
			

			if i == idxName {
				res.Name = val
			}
			if i == idxNs {
				res.Namespace = val
			}
			if i == idxStatus {
				res.Status = val
			}
			if i == idxAge {
				res.Age = val
			}
			if i < len(res.Extras) {
				res.Extras[i] = val
			}

			if len(val)+2 > widths[i] {
				widths[i] = len(val) + 2
			}
		}

		
		if row.Object.Raw != nil || row.Object.Object != nil {
			if meta, ok := row.Object.Object.(metav1.Object); ok {
				if res.Name == "" {
					res.Name = meta.GetName()
				}
				if res.Namespace == "" {
					res.Namespace = meta.GetNamespace()
				}
				if res.Age == "" {
					res.Age = utils.ComputeAge(&metav1.Time{Time: meta.GetCreationTimestamp().Time})
				}
				res.AgeRaw = meta.GetCreationTimestamp().Time
			}
		}
		if res.Namespace == "" {
			res.Namespace = namespace
		}
		if res.AgeRaw.IsZero() {
			res.AgeRaw = time.Now()
		}

		uiResources = append(uiResources, res)
	}

	
	for i, w := range widths {
		if w > 60 {
			widths[i] = 60
		}
		if w < 10 {
			widths[i] = 10
		}
	}

	g.headers = headers
	g.widths = widths
	return uiResources, nil
}