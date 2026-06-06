package graph

import (
	"bytes"
	"context"
	"flag"
	"fmt"

	"github.com/lucasepe/kctx/internal/cmd"
	"github.com/lucasepe/kctx/internal/engine"
	modelgraph "github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/x/cl"
	"github.com/lucasepe/x/log"
)

var _ cl.Task = (*Command)(nil)

func Task(appName string) cl.Task {
	return &Command{
		appName:   appName,
		namespace: "default",
	}
}

type Command struct {
	appName      string
	namespace    string
	resource     string
	resourceName string
	renderer     string
}

func New(appName string) *Command {
	return Task(appName).(*Command)
}

func (c *Command) Name() string {
	return "graph"
}

func (c *Command) Synopsis() string {
	return "Build a structured graph around a supported resource"
}

func (c *Command) Usage() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%s\n\n", c.Synopsis())
	fmt.Fprint(&w, "USAGE:\n\n")
	fmt.Fprintf(&w, "  %s graph <resource> <name> [--namespace namespace] [--render mermaid|dot]\n\n", c.appName)
	fmt.Fprint(&w, "DESCRIPTION:\n\n")
	fmt.Fprintln(&w, "  Builds a deterministic dependency and ownership graph around a supported")
	fmt.Fprintln(&w, "  Kubernetes resource. JSON is emitted by default; --render emits a graph")
	fmt.Fprintln(&w, "  rendering derived from the same graph model.")
	fmt.Fprintln(&w)
	fmt.Fprint(&w, "EXAMPLES:\n\n")
	fmt.Fprintf(&w, "  %s graph pod api-xyz --namespace payments\n", c.appName)
	fmt.Fprintf(&w, "  %s graph pod api-xyz --namespace payments --render mermaid\n", c.appName)
	fmt.Fprintf(&w, "  %s graph applications.argoproj.io guestbook --namespace argocd --render dot\n\n", c.appName)
	return w.String()
}

func (c *Command) Ctx() context.Context {
	return context.Background()
}

func (c *Command) SetFlags(fs *flag.FlagSet) {
	cmd.CommonNamespacedFlags(fs)
	cmd.RenderFlag(fs)
}

func (c *Command) Execute(ctx context.Context, fs *flag.FlagSet, args ...any) cl.ExitStatus {
	env, err := cmd.EnvFrom(args...)
	if err != nil {
		return cl.ExitFailure
	}
	if err := c.configure(fs); err != nil {
		return env.FailJSON(c.Name(), err)
	}

	eng, err := env.Engine()
	if err != nil {
		return env.FailJSON(c.Name(), err)
	}
	result, err := eng.BuildGraph(ctx, engine.BuildGraphRequest{
		Resource:  c.resource,
		Namespace: c.namespace,
		Name:      c.resourceName,
	})
	if err != nil {
		return env.FailJSON(c.Name(), err, log.String("name", c.resourceName))
	}
	if err := c.render(env, result); err != nil {
		return env.FailJSON(c.Name(), err, log.String("name", c.resourceName))
	}
	return cl.ExitSuccess
}

func (c *Command) configure(fs *flag.FlagSet) error {
	resource, resourceName, err := cmd.ResourceAndName(fs.Args(), c.Usage())
	if err != nil {
		return err
	}
	renderer := cmd.RenderValue(fs)
	if renderer != "" && renderer != "mermaid" && renderer != "dot" {
		return fmt.Errorf("unsupported renderer %q\n%s", renderer, c.Usage())
	}
	c.namespace = cmd.NamespaceValue(fs)
	c.resource = resource
	c.resourceName = resourceName
	c.renderer = renderer
	return nil
}

func (c *Command) render(env *cmd.Env, result *modelgraph.Graph) error {
	switch c.renderer {
	case "":
		return render.JSON(env.Out, result)
	case "mermaid":
		return render.MermaidGraph(env.Out, result)
	case "dot":
		return render.DOTGraph(env.Out, result)
	default:
		return fmt.Errorf("unsupported renderer %q", c.renderer)
	}
}
