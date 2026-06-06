package render

import (
	"fmt"
	"io"

	"github.com/lucasepe/kctx/internal/graph"
)

func DOTGraph(w io.Writer, g *graph.Graph) error {
	if _, err := fmt.Fprintln(w, "digraph G {"); err != nil {
		return err
	}
	for _, node := range g.Nodes {
		if _, err := fmt.Fprintf(w, "  %q [label=%q]\n", node.ID, node.Kind+" "+node.Name); err != nil {
			return err
		}
	}
	for _, edge := range g.Edges {
		if _, err := fmt.Fprintf(w, "  %q -> %q [label=%q]\n", edge.Source, edge.Target, edge.Type); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}
