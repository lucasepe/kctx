package graph

import "sort"

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Status    string            `json:"status,omitempty"`
}

type Edge struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason,omitempty"`
}

func NodeID(kind, namespace, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return kind + "/" + namespace + "/" + name
}

type Builder struct {
	nodes map[string]Node
	edges map[string]Edge
}

func NewBuilder() *Builder {
	return &Builder{
		nodes: map[string]Node{},
		edges: map[string]Edge{},
	}
}

func (b *Builder) AddNode(node Node) Node {
	if node.ID == "" {
		node.ID = NodeID(node.Kind, node.Namespace, node.Name)
	}
	if existing, ok := b.nodes[node.ID]; ok {
		if existing.Labels == nil && node.Labels != nil {
			existing.Labels = copyMap(node.Labels)
		}
		if existing.Status == "" {
			existing.Status = node.Status
		}
		b.nodes[node.ID] = existing
		return existing
	}
	node.Labels = copyMap(node.Labels)
	b.nodes[node.ID] = node
	return node
}

func (b *Builder) AddEdge(edge Edge) {
	if edge.Source == "" || edge.Target == "" || edge.Type == "" {
		return
	}
	key := edge.Type + "\x00" + edge.Source + "\x00" + edge.Target + "\x00" + edge.Reason
	b.edges[key] = edge
}

func (b *Builder) Graph() Graph {
	nodes := make([]Node, 0, len(b.nodes))
	for _, node := range b.nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	edges := make([]Edge, 0, len(b.edges))
	for _, edge := range b.edges {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		return edges[i].Reason < edges[j].Reason
	})

	return Graph{Nodes: nodes, Edges: edges}
}

func copyMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
