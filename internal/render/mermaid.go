package render

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/lucasepe/kctx/internal/graph"
)

var mermaidIdentRe = regexp.MustCompile(`[^A-Za-z0-9_]`)

func MermaidGraph(w io.Writer, g *graph.Graph) error {
	if _, err := fmt.Fprintln(w, "graph TD"); err != nil {
		return err
	}

	ids := map[string]string{}
	used := map[string]bool{}
	for i, node := range g.Nodes {
		ident := mermaidIdentRe.ReplaceAllString(node.ID, "_")
		if ident == "" || ident[0] >= '0' && ident[0] <= '9' {
			ident = fmt.Sprintf("n%d", i)
		}
		if used[ident] {
			ident = fmt.Sprintf("n%d", i)
		}
		ids[node.ID] = ident
		used[ident] = true
		if _, err := fmt.Fprintf(w, "  %s[%s]\n", ident, mermaidLabel(node)); err != nil {
			return err
		}
	}
	if len(g.Edges) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	for _, edge := range g.Edges {
		if _, err := fmt.Fprintf(w, "  %s -->|%s| %s\n", ids[edge.Source], mermaidEscape(edge.Type), ids[edge.Target]); err != nil {
			return err
		}
	}
	return nil
}

func mermaidLabel(node graph.Node) string {
	return mermaidEscape(node.Kind + " " + node.Name)
}

func mermaidEscape(value string) string {
	value = strings.ReplaceAll(value, `"`, "'")
	value = strings.ReplaceAll(value, "[", "(")
	value = strings.ReplaceAll(value, "]", ")")
	value = strings.ReplaceAll(value, "|", "/")
	return value
}
