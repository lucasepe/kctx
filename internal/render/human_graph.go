package render

import (
	"fmt"
	"io"
	"sort"

	"github.com/lucasepe/kctx/internal/graph"
)

func HumanGraph(w io.Writer, g *graph.Graph) error {
	root := findPodNode(g)
	if root.ID == "" {
		return nil
	}
	if _, err := fmt.Fprintln(w, root.ID); err != nil {
		return err
	}

	items := graphItems(g, root.ID)
	for i, item := range items {
		last := i == len(items)-1
		if err := writeGraphItem(w, item, "", last); err != nil {
			return err
		}
	}
	return nil
}

type graphItem struct {
	Text     string
	Child    *graphItem
	Priority int
}

func graphItems(g *graph.Graph, podID string) []graphItem {
	var items []graphItem

	if owner := ownerChainItem(g, podID); owner != nil {
		items = append(items, *owner)
	}
	for _, edge := range g.Edges {
		switch {
		case edge.Type == "scheduled_on" && edge.Target == podID:
			items = append(items, graphItem{Text: "scheduled on " + edge.Source, Priority: 20})
		case edge.Type == "selects" && edge.Target == podID:
			items = append(items, graphItem{Text: "selected by " + edge.Source, Priority: 30})
		case edge.Type == "uses_configmap" && edge.Source == podID:
			items = append(items, graphItem{Text: "uses " + edge.Target, Priority: 40})
		case edge.Type == "uses_secret" && edge.Source == podID:
			items = append(items, graphItem{Text: "uses " + edge.Target, Priority: 50})
		case edge.Type == "mounts_pvc" && edge.Source == podID:
			items = append(items, graphItem{Text: "mounts " + edge.Target, Priority: 60})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].Text < items[j].Text
	})
	return items
}

func ownerChainItem(g *graph.Graph, childID string) *graphItem {
	var owners []string
	for _, edge := range g.Edges {
		if edge.Type == "owns" && edge.Target == childID {
			owners = append(owners, edge.Source)
		}
	}
	sort.Strings(owners)
	if len(owners) == 0 {
		return nil
	}
	ownerID := owners[0]
	item := graphItem{Text: "owned by " + ownerID, Priority: 10}
	item.Child = ownerChainItem(g, ownerID)
	return &item
}

func writeGraphItem(w io.Writer, item graphItem, prefix string, last bool) error {
	branch := "├── "
	nextPrefix := prefix + "│   "
	if last {
		branch = "└── "
		nextPrefix = prefix + "    "
	}
	if _, err := fmt.Fprintln(w, prefix+branch+item.Text); err != nil {
		return err
	}
	if item.Child != nil {
		return writeGraphItem(w, *item.Child, nextPrefix, true)
	}
	return nil
}

func findPodNode(g *graph.Graph) graph.Node {
	for _, node := range g.Nodes {
		if node.Kind == "Pod" {
			return node
		}
	}
	return graph.Node{}
}
