package tree

import "fmt"



type NodeKind int
type EdgeType int
type Tier int
type VisualLevel int 

const (
	
	LevelRoot       VisualLevel = iota 
	LevelCritical                      
	LevelPeripheral                    
)

const (
	
	KindWorkload NodeKind = iota
	KindService
	KindNetwork 
	KindConfig  
	KindInfra   
	KindExternal
	KindGroup   
	KindUnknown
)

const (
	
	EdgeUses       EdgeType = iota 
	EdgeExposedBy          
	EdgeConfiguredBy      
	EdgeDependsOn          
	EdgeOwns               
)

const (
	
	TierCritical Tier = iota 
	TierHigh
	TierStandard
	TierLow
)


type TopologyNode struct {
	
	ID        string
	Kind      NodeKind
	Name      string
	Namespace string
	Icon      string
	
	
	Status    string   
	Details   []string 
	
	
	Criticality Tier
	RiskScore   int 

	
	Level       VisualLevel 
	IsGroup     bool        
	GroupCount  int         

	
	X, Y     int
	Width    int
	Height   int
	ColIndex int 
	LevelIdx int 
}


type TopologyEdge struct {
	FromID string
	ToID   string
	Type   EdgeType
	Risk   int 
}


type TopologyGraph struct {
	Nodes  map[string]*TopologyNode
	Edges  []TopologyEdge
	RootID string
}


func NewTopologyGraph(rootID string) *TopologyGraph {
	return &TopologyGraph{
		Nodes:  make(map[string]*TopologyNode),
		Edges:  make([]TopologyEdge, 0),
		RootID: rootID,
	}
}


func (g *TopologyGraph) AddNode(n *TopologyNode) {
	if n == nil {
		return
	}
	g.Nodes[n.ID] = n
}


func (g *TopologyGraph) AddEdge(from, to string, rel EdgeType) {
	g.Edges = append(g.Edges, TopologyEdge{
		FromID: from, 
		ToID:   to, 
		Type:   rel,
	})
}


func (n *TopologyNode) String() string {
	return fmt.Sprintf("[%s] %s (%s)", n.Kind, n.Name, n.Status)
}



type Node struct {
	Kind     string   
	Name     string   
	Icon     string   
	Status   string   
	Details  string   
	Children []*Node
	Expanded bool     
	Parent   *Node    

	
	depth  int
	isLast bool
}


func NewNode(kind, name, icon, status string) *Node {
	return &Node{
		Kind:     kind,
		Name:     name,
		Icon:     icon,
		Status:   status,
		Children: make([]*Node, 0),
		Expanded: false,
	}
}


func NewVirtualNode(name, icon string) *Node {
	return &Node{
		Kind:     "Virtual", 
		Name:     name,
		Icon:     icon,
		Children: make([]*Node, 0),
		Expanded: true, 
	}
}


func (n *Node) AddChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}


func (n *Node) SetDepth(d int) {
	n.depth = d
}


func (n *Node) GetDepth() int {
	return n.depth
}


func (n *Node) SetLast(l bool) {
	n.isLast = l
}


func (n *Node) IsLast() bool {
	return n.isLast
}